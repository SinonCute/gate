package proxy

import (
	"fmt"
	"net"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/state/states"

	"gate/pkg/edition/java/config"
	"gate/pkg/edition/java/internal/addrquota"
	"gate/pkg/edition/java/lite"
	"gate/pkg/edition/java/netmc"
	"gate/pkg/edition/java/proto/packet"
	"gate/pkg/edition/java/proto/state"
	"gate/pkg/gate/proto"
	"gate/pkg/util/netutil"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
)

type sessionHandlerDeps struct {
	proxy          *Proxy
	eventMgr       event.Manager
	configProvider configProvider
	loginsQuota    *addrquota.Quota
}

func (d *sessionHandlerDeps) config() *config.Config {
	return d.configProvider.config()
}

type handshakeSessionHandler struct {
	*sessionHandlerDeps

	conn netmc.MinecraftConn
	log  logr.Logger

	nopSessionHandler
}

// newHandshakeSessionHandler returns a handler used for clients in the handshake state.
func newHandshakeSessionHandler(
	conn netmc.MinecraftConn,
	deps *sessionHandlerDeps,
) netmc.SessionHandler {
	return &handshakeSessionHandler{
		sessionHandlerDeps: deps,
		conn:               conn,
		log:                logr.FromContextOrDiscard(conn.Context()).WithName("handshakeSession"),
	}
}

func (h *handshakeSessionHandler) HandlePacket(p *proto.PacketContext) {
	if !p.KnownPacket() {
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
		return
	}
	switch typed := p.Packet.(type) {
	// TODO legacy pings
	case *packet.Handshake:
		h.handleHandshake(typed, p)
	default:
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
	}
}

func (h *handshakeSessionHandler) handleHandshake(handshake *packet.Handshake, pc *proto.PacketContext) {
	// The client sends the next wanted state in the Handshake packet.
	nextState := stateForProtocol(handshake.NextStatus)
	if nextState == nil {
		h.log.V(1).Info("client provided invalid next status state, closing connection",
			"nextStatus", handshake.NextStatus)
		_ = h.conn.Close()
		return
	}

	// Update connection to requested state and protocol sent in the packet.
	h.conn.SetProtocol(proto.Protocol(handshake.ProtocolVersion))

	// Lite mode ping resolver
	var resolvePingResponse pingResolveFunc
	if h.config().Lite.Enabled {
		h.conn.SetState(nextState)
		dialTimeout := time.Duration(h.config().ConnectionTimeout)
		if nextState == state.Login {
			// Lite mode enabled, pipe the connection.
			lite.Forward(dialTimeout, h.config().Lite.Routes, h.log, h.conn, handshake, pc)
			return
		}
		// Resolve ping response for lite mode.
		resolvePingResponse = func(log logr.Logger, statusRequestCtx *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error) {
			return lite.ResolveStatusResponse(dialTimeout, h.config().Lite.Routes, log, h.conn, handshake, pc, statusRequestCtx)
		}
	}

	vHost := netutil.NewAddr(
		fmt.Sprintf("%s:%d", handshake.ServerAddress, handshake.Port),
		h.conn.LocalAddr().Network(),
	)
	handshakeIntent := handshake.Intent()
	inbound := newInitialInbound(h.conn, vHost, handshakeIntent)

	if handshakeIntent == packet.TransferHandshakeIntent && !h.config().AcceptTransfers {
		_ = inbound.disconnect(&component.Translation{Key: "multiplayer.disconnect.transfers_disabled"})
		return
	}

	switch nextState {
	case state.Status:
		// Client wants to enter the Status state to get the server status.
		// Just update the session handler and wait for the StatusRequest packet.
		handler := newStatusSessionHandler(h.conn, inbound, h.sessionHandlerDeps, resolvePingResponse)
		h.conn.SetActiveSessionHandler(state.Status, handler)
	}
}

func stateForProtocol(status int) *state.Registry {

	switch states.State(status) {
	case states.StatusState:
		return state.Status
	case states.LoginState, states.State(packet.TransferHandshakeIntent):
		return state.Login
	}
	return nil
}

type initialInbound struct {
	netmc.MinecraftConn
	virtualHost     net.Addr
	handshakeIntent packet.HandshakeIntent
}

var _ Inbound = (*initialInbound)(nil)

func newInitialInbound(c netmc.MinecraftConn, virtualHost net.Addr, handshakeIntent packet.HandshakeIntent) *initialInbound {
	return &initialInbound{
		MinecraftConn:   c,
		virtualHost:     virtualHost,
		handshakeIntent: handshakeIntent,
	}
}

func (i *initialInbound) VirtualHost() net.Addr {
	return i.virtualHost
}

func (i *initialInbound) HandshakeIntent() packet.HandshakeIntent {
	return i.handshakeIntent
}

func (i *initialInbound) Active() bool {
	return !netmc.Closed(i.MinecraftConn)
}

func (i *initialInbound) String() string {
	return fmt.Sprintf("[initial connection] %s -> %s", i.RemoteAddr(), i.virtualHost)
}

func (i *initialInbound) disconnect(reason component.Component) error {
	// TODO add cfg option to log player connections to log "player disconnected"
	return netmc.CloseWith(i.MinecraftConn, packet.NewDisconnect(reason, i.Protocol(), i.State().State))
}

//
//
//
//
//
//

// A no-operation session handler can be wrapped to
// implement the sessionHandler interface.
type nopSessionHandler struct{}

var _ netmc.SessionHandler = (*nopSessionHandler)(nil)

func (nopSessionHandler) HandlePacket(*proto.PacketContext) {}
func (nopSessionHandler) Disconnected()                     {}
func (nopSessionHandler) Deactivated()                      {}
func (nopSessionHandler) Activated()                        {}
