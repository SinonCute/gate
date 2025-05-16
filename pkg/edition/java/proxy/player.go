package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robinbraemer/event"

	cfgpacket "go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/internal/future"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/sets"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/permission"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Player is a connected Minecraft player.
type Player interface { // TODO convert to struct(?) bc this is a lot of methods and only *connectedPlayer implements it
	Inbound
	netmc.PacketWriter

	ID() uuid.UUID    // The Minecraft ID of the player.
	Username() string // The username of the player.
	// CurrentServer returns the current server connection of the player.
	Ping() time.Duration // The player's ping or -1 if currently unknown.
	// Disconnect disconnects the player with a reason.
	// Once called, further interface calls to this player become undefined.
	Disconnect(reason component.Component)
	// SendActionBar sends an action bar to the player.
	SendActionBar(msg component.Component) error
	ClientBrand() string // Returns the player's client brand. Empty if unspecified.
	// TransferToHost transfers the player to the specified host.
	// The host should be in the format of "host:port" or just "host" in which case the port defaults to 25565.
	// If the player is from a version lower than 1.20.5, this method will return ErrTransferUnsupportedClientProtocol.
	TransferToHost(addr string) error
}

type connectedPlayer struct {
	netmc.MinecraftConn
	*sessionHandlerDeps

	log             logr.Logger
	virtualHost     net.Addr
	ping            atomic.Duration
	profile         *profile.GameProfile
	permFunc        permission.Func
	handshakeIntent packet.HandshakeIntent
	// This field is true if this connection is being disconnected
	// due to another connection logging in with the same GameProfile.
	disconnectDueToDuplicateConnection atomic.Bool
	clientsideChannels                 *sets.CappedSet[string]
	pendingConfigurationSwitch         bool

	mu                   sync.RWMutex // Protects following fields
	clientSettingsPacket *packet.ClientSettings

	clientBrand string // may be empty
}

var _ Player = (*connectedPlayer)(nil)

const maxClientsidePluginChannels = 1024

func newConnectedPlayer(
	conn netmc.MinecraftConn,
	profile *profile.GameProfile,
	virtualHost net.Addr,
	handshakeIntent packet.HandshakeIntent,
	sessionHandlerDeps *sessionHandlerDeps,
) *connectedPlayer {
	var ping atomic.Duration
	ping.Store(-1)

	p := &connectedPlayer{
		sessionHandlerDeps: sessionHandlerDeps,
		MinecraftConn:      conn,
		log: logr.FromContextOrDiscard(conn.Context()).WithName("player").WithValues(
			"name", profile.Name, "id", profile.ID),
		profile:            profile,
		virtualHost:        virtualHost,
		handshakeIntent:    handshakeIntent,
		clientsideChannels: sets.NewCappedSet[string](maxClientsidePluginChannels),
		ping:               ping,
		permFunc:           func(string) permission.TriState { return permission.Undefined },
	}
	return p
}

func (p *connectedPlayer) Ping() time.Duration {
	return p.ping.Load()
}

func (p *connectedPlayer) GameProfile() profile.GameProfile {
	return *p.profile
}

var (
	ErrNoBackendConnection = errors.New("player has no backend server connection yet")
	ErrTooLongChatMessage  = errors.New("server bound chat message can not exceed 256 characters")
)

func (p *connectedPlayer) VirtualHost() net.Addr {
	return p.virtualHost
}

func (p *connectedPlayer) Active() bool {
	return !netmc.Closed(p.MinecraftConn)
}

// WithMessageSender modifies the sender identity of the chat message.
func WithMessageSender(id uuid.UUID) command.MessageOption {
	return messageApplyOption(func(o any) {
		if b, ok := o.(*chat.Builder); ok {
			b.Sender = id
		}
	})
}

// MessageType is a chat message type.
type MessageType = chat.MessageType

// Chat message types.
const (
	// ChatMessageType is a standard chat message and
	// lets the chat message appear in the client's HUD.
	// These messages can be filtered out by the client's settings.
	ChatMessageType = chat.ChatMessageType
	// SystemMessageType is a system chat message.
	// e.g. client is willing to accept messages from commands,
	// but does not want general chat from other players.
	// It lets the chat message appear in the client's HUD and can't be dismissed.
	SystemMessageType = chat.SystemMessageType
	// GameInfoMessageType lets the chat message appear above the player's main HUD.
	// This text format doesn't support many component features, such as hover events.
	GameInfoMessageType = chat.GameInfoMessageType
)

// WithMessageType modifies chat message type.
func WithMessageType(t MessageType) command.MessageOption {
	return messageApplyOption(func(o any) {
		if b, ok := o.(*chat.Builder); ok {
			if t != ChatMessageType {
				t = SystemMessageType
			}
			b.Type = t
		}
	})
}

type messageApplyOption func(o any)

func (a messageApplyOption) Apply(o any) { a(o) }

func (p *connectedPlayer) SendMessage(msg component.Component, opts ...command.MessageOption) error {
	if msg == nil {
		return nil // skip nil message
	}
	b := chat.Builder{
		Protocol:  p.Protocol(),
		Type:      ChatMessageType,
		Sender:    p.ID(),
		Component: msg,
	}
	for _, o := range opts {
		o.Apply(b)
	}
	return p.WritePacket(b.ToClient())
}

var legacyJsonCodec = &legacy.Legacy{}

func (p *connectedPlayer) SendActionBar(msg component.Component) error {
	if msg == nil {
		return nil // skip nil message
	}
	protocol := p.Protocol()
	if protocol.GreaterEqual(version.Minecraft_1_11) {
		// Use the title packet instead.
		pkt, err := title.New(protocol, &title.Builder{
			Action:    title.SetActionBar,
			Component: *chat.FromComponent(msg),
		})
		if err != nil {
			return err
		}
		return p.WritePacket(pkt)
	}

	// Due to issues with action bar packets, we'll need to convert the text message into a
	// legacy message and then put the legacy text into a component... (╯°□°)╯︵ ┻━┻!
	b := new(strings.Builder)
	if err := legacyJsonCodec.Marshal(b, msg); err != nil {
		return err
	}
	m, err := json.Marshal(map[string]string{"text": b.String()})
	if err != nil {
		return err
	}
	return p.WritePacket(&chat.LegacyChat{
		Message: string(m),
		Type:    chat.GameInfoMessageType,
		Sender:  uuid.Nil,
	})
}

func (p *connectedPlayer) Username() string { return p.profile.Name }

func (p *connectedPlayer) ID() uuid.UUID { return p.profile.ID }

func (p *connectedPlayer) Disconnect(reason component.Component) {
	if !p.Active() {
		return
	}

	var r string
	b := new(strings.Builder)
	if (&legacy.Legacy{}).Marshal(b, reason) == nil {
		r = b.String()
	}

	if netmc.CloseWith(p, packet.NewDisconnect(reason, p.Protocol(), p.State().State)) == nil {
		p.log.Info("player has been disconnected", "reason", r)
	}
}

func (p *connectedPlayer) String() string { return p.profile.Name }

// ClientSettingsPacket returns the last known client settings packet.
// If not known already, returns nil.
func (p *connectedPlayer) ClientSettingsPacket() *packet.ClientSettings {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clientSettingsPacket
}

func (p *connectedPlayer) config() *config.Config {
	return p.configProvider.config()
}

// switchToConfigState switches the connection of the client into config state.
func (p *connectedPlayer) switchToConfigState() {
	if err := p.BufferPacket(new(cfgpacket.StartUpdate)); err != nil {
		p.log.Error(err, "error writing config packet")
	}

	p.pendingConfigurationSwitch = true
	p.MinecraftConn.Writer().SetState(state.Config)
	// Make sure we don't send any play packets to the player after update start
	p.MinecraftConn.EnablePlayPacketQueue()

	_ = p.Flush() // Trigger switch finally
}

func (p *connectedPlayer) ClientBrand() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clientBrand
}

// setClientBrand sets the client brand of the player.
func (p *connectedPlayer) setClientBrand(brand string) {
	p.mu.Lock()
	p.clientBrand = brand
	p.mu.Unlock()
}

var ErrTransferUnsupportedClientProtocol = errors.New("player version must be 1.20.5 to be able to transfer to another host")

func (p *connectedPlayer) TransferToHost(addr string) error {
	if strings.TrimSpace(addr) == "" {
		return errors.New("empty address")
	}
	if p.Protocol().Lower(version.Minecraft_1_20_5) {
		return fmt.Errorf("%w: but player is on %s", ErrTransferUnsupportedClientProtocol, p.Protocol())
	}

	host, port, err := net.SplitHostPort(addr)
	var portInt int
	if err != nil {
		host = addr
		portInt = 25565
	} else {
		portInt, err = strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port, err)
		}
	}

	targetAddr := netutil.NewAddr(fmt.Sprintf("%s:%d", host, portInt), "tcp")
	f := future.NewChan[error]()
	event.FireParallel(p.eventMgr, newPreTransferEvent(p, targetAddr), func(e *PreTransferEvent) {
		defer f.Complete(nil)
		if e.Allowed() {
			resultedAddr := e.Addr()
			if resultedAddr != nil {
				resultedAddr = targetAddr
			}
			host, port := netutil.HostPort(resultedAddr)
			err = p.WritePacket(&packet.Transfer{
				Host: host,
				Port: int(port),
			})
			if err != nil {
				f.Complete(err)
				return
			}
			p.log.Info("transferring player to host", "host", resultedAddr)
		}
	})
	return f.Get()
}

func (p *connectedPlayer) HandshakeIntent() packet.HandshakeIntent {
	return p.handshakeIntent
}
