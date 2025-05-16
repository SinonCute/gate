package proxy

import (
	"errors"
	"fmt"
	"gate/pkg/internal/future"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/sets"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Player is a connected Minecraft player.
type Player interface { // TODO convert to struct(?) bc this is a lot of methods and only *connectedPlayer implements it
	Inbound
	netmc.PacketWriter

	ID() uuid.UUID    // The Minecraft ID of the player.
	Username() string // The username of the player.
	Connection() netmc.MinecraftConn
	Ping() time.Duration // The player's ping or -1 if currently unknown.
	// Disconnect disconnects the player with a reason.
	// Once called, further interface calls to this player become undefined.
	Disconnect(reason component.Component)
	ClientBrand() string // Returns the player's client brand. Empty if unspecified.
	// TransferToHost transfers the player to the specified host.
	// The host should be in the format of "host:port" or just "host" in which case the port defaults to 25565.
	// If the player is from a version lower than 1.20.5, this method will return ErrTransferUnsupportedClientProtocol.
	TransferToHost(addr string) error
	SetCurrentBackendServerAddress(addr net.Addr)
	CurrentBackendServerAddress() string
}

type connectedPlayer struct {
	netmc.MinecraftConn
	*sessionHandlerDeps

	log             logr.Logger
	virtualHost     net.Addr
	ping            atomic.Duration
	profile         *profile.GameProfile
	handshakeIntent packet.HandshakeIntent
	// This field is true if this connection is being disconnected
	// due to another connection logging in with the same GameProfile.
	disconnectDueToDuplicateConnection atomic.Bool
	clientsideChannels                 *sets.CappedSet[string]
	pendingConfigurationSwitch         bool

	mu                   sync.RWMutex // Protects following fields
	clientSettingsPacket *packet.ClientSettings

	clientBrand string // may be empty

	currentBackendServerAddress atomic.Value // Stores string representation of net.Addr
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
	}
	return p
}

func (p *connectedPlayer) Connection() netmc.MinecraftConn {
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

type messageApplyOption func(o any)

func (a messageApplyOption) Apply(o any) { a(o) }

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

func (p *connectedPlayer) SetCurrentBackendServerAddress(addr net.Addr) {
	if addr == nil {
		p.currentBackendServerAddress.Store("")
	} else {
		p.currentBackendServerAddress.Store(addr.String())
	}
}

func (p *connectedPlayer) CurrentBackendServerAddress() string {
	s, _ := p.currentBackendServerAddress.Load().(string)
	return s
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

	hostTarget, portTarget := netutil.HostPort(targetAddr)
	err = p.WritePacket(&packet.Transfer{
		Host: hostTarget,
		Port: int(portTarget),
	})
	if err != nil {
		f.Complete(err)
		return f.Get()
	}
	p.log.Info("transferring player to host", "host", targetAddr)

	return f.Get()
}

func (p *connectedPlayer) HandshakeIntent() packet.HandshakeIntent {
	return p.handshakeIntent
}
