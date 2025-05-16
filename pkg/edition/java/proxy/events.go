package proxy

import (
	"net"

	"go.minekube.com/gate/pkg/util/uuid"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/util/permission"
)

// ConnectionEventConn tracks whether Close was called on the connection.
type ConnectionEventConn interface {
	net.Conn
	Closed() bool // Whether Close was called.
}

// ConnectionEvent is fired when a client connects with the proxy.
// It can be used for low-level connection handling, modifying or closing the connection.
// This event is fired before ConnectionHandshakeEvent
type ConnectionEvent struct {
	conn     net.Conn
	original ConnectionEventConn
}

// Connection returns the connection.
func (e *ConnectionEvent) Connection() net.Conn {
	return e.conn
}

// OriginalConnection returns the original connection.
func (e *ConnectionEvent) OriginalConnection() ConnectionEventConn {
	return e.original
}

// SetConnection sets new connection to use for this connection.
func (e *ConnectionEvent) SetConnection(conn net.Conn) {
	e.conn = conn
}

//
//
//
//
//

// ConnectionHandshakeEvent is fired when a handshake
// is established between a client and the proxy.
type ConnectionHandshakeEvent struct {
	inbound Inbound
	intent  packet.HandshakeIntent
}

// Connection returns the inbound connection.
func (e *ConnectionHandshakeEvent) Connection() Inbound {
	return e.inbound
}

// Intent returns the handshake intent.
func (e *ConnectionHandshakeEvent) Intent() packet.HandshakeIntent {
	return e.intent
}

//
//
//
//
//

// GameProfileRequestEvent is fired after the PreLoginEvent in
// order to set up the game profile for the user.
// This can be used to configure a custom profile for a user, i.e. skin replacement.
type GameProfileRequestEvent struct {
	inbound    Inbound
	original   profile.GameProfile
	onlineMode bool

	use profile.GameProfile
}

// NewGameProfileRequestEvent creates a new GameProfileRequestEvent.
func NewGameProfileRequestEvent(
	inbound Inbound,
	original profile.GameProfile,
	onlineMode bool,
) *GameProfileRequestEvent {
	return &GameProfileRequestEvent{
		inbound:    inbound,
		original:   original,
		onlineMode: onlineMode,
	}
}

// Conn returns the inbound connection that is connecting to the proxy.
func (e *GameProfileRequestEvent) Conn() Inbound {
	return e.inbound
}

// Original returns the by the proxy created offline or online (Mojang authenticated) game profile.
func (e *GameProfileRequestEvent) Original() profile.GameProfile {
	return e.original
}

// OnlineMode specifies whether the user connected in online/offline mode.
func (e *GameProfileRequestEvent) OnlineMode() bool {
	return e.onlineMode
}

// SetGameProfile sets the profile to use for this connection.
func (e *GameProfileRequestEvent) SetGameProfile(p profile.GameProfile) {
	e.use = p
}

// GameProfile returns the game profile that will be used to initialize the connection with.
// Should no profile be set, the original profile (given by the proxy) will be used.
func (e *GameProfileRequestEvent) GameProfile() profile.GameProfile {
	if len(e.use.Name) == 0 {
		return e.original
	}
	return e.use
}

//
//
//
//
//
//
//
//

// PermissionsSetupEvent is fired once a permission.Subject's
// permissions are being initialized.
type PermissionsSetupEvent struct {
	subject     permission.Subject
	defaultFunc permission.Func

	fn permission.Func
}

// Subject returns the subject the permissions are setup for.
func (p *PermissionsSetupEvent) Subject() permission.Subject {
	return p.subject
}

// Func returns the permission.Func used for the subject.
func (p *PermissionsSetupEvent) Func() permission.Func {
	if p.fn == nil {
		return p.defaultFunc
	}
	return p.fn
}

// SetFunc sets the permission.Func usec for the subject.
// If fn is nil, the default Func fill be used.
func (p *PermissionsSetupEvent) SetFunc(fn permission.Func) {
	if fn == nil {
		return
	}
	p.fn = fn
}

//
//
//
//
//
//
//

// PreLoginEvent is fired when a player has initiated a connection with the proxy
// but before the proxy authenticates the player with Mojang or before the player's proxy connection
// is fully established (for offline mode).
type PreLoginEvent struct {
	connection Inbound
	username   string
	id         uuid.UUID // player's uuid, nil-able

	result PreLoginResult
	reason component.Component
}

func newPreLoginEvent(conn Inbound, username string, id uuid.UUID) *PreLoginEvent {
	return &PreLoginEvent{
		connection: conn,
		username:   username,
		id:         id,
		result:     AllowedPreLogin,
	}
}

// PreLoginResult is the result of a PreLoginEvent.
type PreLoginResult uint8

// PreLoginResult values.
const (
	AllowedPreLogin PreLoginResult = iota
	DeniedPreLogin
	ForceOnlineModePreLogin
	ForceOfflineModePreLogin
)

// Username returns the username of the player.
func (e *PreLoginEvent) Username() string {
	return e.username
}

// ID returns the UUID of the connecting player. May be uuid.Nil!
// This value is nil on 1.19.2 and lower,
// up to 1.20.1 it is optional and from 1.20.2 it will always be available.
func (e *PreLoginEvent) ID() (uuid.UUID, bool) {
	return e.id, e.id != uuid.Nil
}

// Conn returns the inbound connection that is connecting to the proxy.
func (e *PreLoginEvent) Conn() Inbound {
	return e.connection
}

// Result returns the current result of the PreLoginEvent.
func (e *PreLoginEvent) Result() PreLoginResult {
	return e.result
}

// Reason returns the `deny reason` to disconnect the connection.
// May be nil!
func (e *PreLoginEvent) Reason() component.Component {
	return e.reason
}

func (e *PreLoginEvent) Deny(reason component.Component) {
	e.result = DeniedPreLogin
	e.reason = reason
}

func (e *PreLoginEvent) Allow() {
	e.result = AllowedPreLogin
	e.reason = nil
}

func (e *PreLoginEvent) ForceOnlineMode() {
	e.result = ForceOnlineModePreLogin
	e.reason = nil
}

func (e *PreLoginEvent) ForceOfflineMode() {
	e.result = ForceOfflineModePreLogin
	e.reason = nil
}

//
//
//
//
//
//
//
//

type LoginEvent struct {
	player Player

	denied bool
	reason component.Component
}

func (e *LoginEvent) Player() Player {
	return e.player
}

func (e *LoginEvent) Deny(reason component.Component) {
	e.denied = true
	e.reason = reason
}

func (e *LoginEvent) Allow() {
	e.denied = false
	e.reason = nil
}

func (e *LoginEvent) Allowed() bool {
	return !e.denied
}

// Is nil if Allowed() returns true
func (e *LoginEvent) Reason() component.Component {
	return e.reason
}

//
//
//
//
//
//
//

type DisconnectEvent struct {
	player      Player
	loginStatus LoginStatus
}

type LoginStatus uint8

const (
	SuccessfulLoginStatus LoginStatus = iota
	ConflictingLoginStatus
	CanceledByUserLoginStatus
	CanceledByProxyLoginStatus
	CanceledByUserBeforeCompleteLoginStatus
)

func (e *DisconnectEvent) Player() Player {
	return e.player
}

func (e *DisconnectEvent) LoginStatus() LoginStatus {
	return e.loginStatus
}

//
//
//
//
//
//
//
//

type PostLoginEvent struct {
	player Player
}

func (e *PostLoginEvent) Player() Player {
	return e.player
}

//
//
//
//
//
//

// PlayerChooseInitialServerEvent is fired when a player has finished the login process,
// and we need to choose the first server to connect to.
// The proxy will wait on this event to finish firing before initiating the connection
// but you should try to limit the work done in this event.
// Failures will be handled by KickedFromServerEvent as normal.
type PlayerChooseInitialServerEvent struct {
	player        Player
	initialServer RegisteredServer // May be nil if no server is configured.
}

// Player returns the player to find the initial server for.
func (e *PlayerChooseInitialServerEvent) Player() Player {
	return e.player
}

// InitialServer returns the initial server or nil if no server is configured.
func (e *PlayerChooseInitialServerEvent) InitialServer() RegisteredServer {
	return e.initialServer
}

// SetInitialServer sets the initial server for the player.
func (e *PlayerChooseInitialServerEvent) SetInitialServer(server RegisteredServer) {
	e.initialServer = server
}

//
//
//
//
//
//

// ServerPreConnectEvent is fired before the player connects to a server.
type ServerPreConnectEvent struct {
	player   Player
	original RegisteredServer

	server RegisteredServer
}

func newServerPreConnectEvent(player Player, server RegisteredServer) *ServerPreConnectEvent {
	return &ServerPreConnectEvent{
		player:   player,
		original: server,
		server:   server,
	}
}

// Player returns the player that tries to connect to another server.
func (e *ServerPreConnectEvent) Player() Player {
	return e.player
}

// OriginalServer returns the server that the player originally tried to connect to.
// To get the server the player will connect to, see the Server() of this event.
// To get the server the player is currently on when this event is fired, use Player.getCurrentServer().
func (e *ServerPreConnectEvent) OriginalServer() RegisteredServer {
	return e.original
}

// Allow the player to connect to the specified server.
func (e *ServerPreConnectEvent) Allow(server RegisteredServer) {
	e.server = server
}

// Deny will cancel the player to connect to another server.
func (e *ServerPreConnectEvent) Deny() {
	e.server = nil
}

// Allowed returns true whether the connection is allowed.
func (e *ServerPreConnectEvent) Allowed() bool {
	return e.server != nil
}

// Server returns the server the player will connect to OR
// nil if Allowed() returns false.
func (e *ServerPreConnectEvent) Server() RegisteredServer {
	return e.server
}

//
//
//
//
//
//

// PreTransferEvent is fired before a player is transferred to another host,
// either by the backend server or by a plugin using the Player.TransferTo method.
type PreTransferEvent struct {
	player       Player
	originalAddr net.Addr
	targetAddr   net.Addr
	denied       bool
}

func newPreTransferEvent(player Player, addr net.Addr) *PreTransferEvent {
	return &PreTransferEvent{
		player:       player,
		originalAddr: addr,
		targetAddr:   addr,
	}
}

// TransferTo changes the target address the player will be transferred to.
func (e *PreTransferEvent) TransferTo(addr net.Addr) {
	e.targetAddr = addr
	e.denied = false
}

// Addr returns the target address the player will be transferred to.
func (e *PreTransferEvent) Addr() net.Addr {
	return e.targetAddr
}

// Allowed returns true if the transfer is allowed.
func (e *PreTransferEvent) Allowed() bool {
	return !e.denied
}

// Player returns the player that is about to be transferred.
func (e *PreTransferEvent) Player() Player {
	return e.player
}

//
//
//
//
//
//

// Fired when a player is kicked from a server. You may either allow the proxy to kick the player
// (with an optional reason override) or redirect the player to a separate server. By default,
// the proxy will notify the user (if they are already connected to a server) or disconnect them
// (if they are not on a server and no other servers are available).
type KickedFromServerEvent struct {
	player              Player
	server              RegisteredServer
	originalReason      component.Component // May be nil!
	duringServerConnect bool

	result ServerKickResult
}

// ServerKickResult is the result of a KickedFromServerEvent and is implemented by
//
// # DisconnectPlayerKickResult
//
// # RedirectPlayerKickResult
//
// NotifyKickResult
type ServerKickResult interface {
	isServerKickResult() // assert implemented internally
}

var (
	_ ServerKickResult = (*DisconnectPlayerKickResult)(nil)
	_ ServerKickResult = (*RedirectPlayerKickResult)(nil)
	_ ServerKickResult = (*NotifyKickResult)(nil)
)

func newKickedFromServerEvent(
	player Player, server RegisteredServer,
	reason component.Component, duringServerConnect bool,
	initialResult ServerKickResult,
) *KickedFromServerEvent {
	return &KickedFromServerEvent{
		player:              player,
		server:              server,
		originalReason:      reason,
		duringServerConnect: duringServerConnect,
		result:              initialResult,
	}
}

// Player returns the player that got kicked.
func (e *KickedFromServerEvent) Player() Player {
	return e.player
}

// Server returns the server the player got kicked from.
func (e *KickedFromServerEvent) Server() RegisteredServer {
	return e.server
}

// OriginalReason returns the reason the server kicked the player from the server.
// May return nil!
func (e *KickedFromServerEvent) OriginalReason() component.Component {
	return e.originalReason
}

// KickedDuringServerConnect returns true if the player got kicked while connecting to another server.
func (e *KickedFromServerEvent) KickedDuringServerConnect() bool {
	return e.duringServerConnect
}

// Result returns current kick result.
// The proxy sets a default non-nil result but an event handler
// may has set it nil when handling the event.
func (e *KickedFromServerEvent) Result() ServerKickResult {
	return e.result
}

// SetResult sets the kick result.
func (e *KickedFromServerEvent) SetResult(result ServerKickResult) {
	e.result = result
}

// DisconnectPlayerKickResult is a ServerKickResult and
// tells the proxy to disconnect the player with the specified reason.
type DisconnectPlayerKickResult struct {
	Reason component.Component
}

func (*DisconnectPlayerKickResult) isServerKickResult() {}

// RedirectPlayerKickResult is a ServerKickResult and
// tells the proxy to redirect the player to another server.
type RedirectPlayerKickResult struct {
	Server  RegisteredServer    // The new server to redirect the kicked player to.
	Message component.Component // Optional message sent to the player after redirecting.
}

func (*RedirectPlayerKickResult) isServerKickResult() {}

// NotifyKickResult is ServerKickResult and
// notifies the player with the specified message but does nothing else.
// This is only a valid result to use if the player was trying to connect
// to a different server, otherwise it is treated like a DisconnectPlayerKickResult result.
type NotifyKickResult struct {
	Message component.Component
}

func (*NotifyKickResult) isServerKickResult() {}

//
//
//
//

// PlayerClientBrandEvent is fired when a Player sends the `minecraft:brand` plugin message.
// The proxy will not wait on event handlers to finish firing.
type PlayerClientBrandEvent struct {
	player Player
	brand  string
}

func (e *PlayerClientBrandEvent) Player() Player {
	return e.player
}
func (e *PlayerClientBrandEvent) Brand() string {
	return e.brand
}

//
//
//
//

// PreShutdownEvent is fired before the proxy begins to shut down by
// stopping to accept new connections and disconnect all players.
type PreShutdownEvent struct {
	reason component.Component // may be nil
}

// Reason returns the shutdown reason used to disconnect players with.
// May be nil!
func (s *PreShutdownEvent) Reason() component.Component {
	return s.reason
}

// SetReason sets the shutdown reason used to disconnect players with.
func (s *PreShutdownEvent) SetReason(reason component.Component) {
	s.reason = reason
}

//
//
//
//

// ReadyEvent is fired once the proxy was successfully
// initialized and is ready to serve connections.
//
// May be triggered multiple times on config reloads.
type ReadyEvent struct {
	addr string // The address the proxy is listening on.
}

// Addr returns the address the proxy is listening on.
func (r *ReadyEvent) Addr() string { return r.addr }

// ShutdownEvent is fired by the proxy after the proxy
// has stopped accepting connections and PreShutdownEvent,
// but before the proxy process exits.
//
// Subscribe to this event to gracefully stop any subtasks,
// such as plugin dependencies.
type ShutdownEvent struct{}
