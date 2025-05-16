package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gate/pkg/edition/java/config"
	"gate/pkg/edition/java/internal/addrquota"
	"gate/pkg/edition/java/internal/reload"
	"gate/pkg/edition/java/lite"
	"gate/pkg/edition/java/netmc"
	"gate/pkg/edition/java/proto/state"
	"gate/pkg/gate/proto"
	"gate/pkg/util/errs"
	"gate/pkg/util/netutil"
	"gate/pkg/util/uuid"

	"github.com/go-logr/logr"
	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"golang.org/x/sync/errgroup"
)

// Proxy is Gate's Java edition Minecraft proxy.
type Proxy struct {
	log   logr.Logger
	cfg   *config.Config
	event event.Manager

	startTime atomic.Pointer[time.Time]

	closeMu     sync.Mutex
	startCtx    context.Context
	cancelStart context.CancelFunc
	started     bool

	muS sync.RWMutex // Protects following field

	muP         sync.RWMutex                   // Protects following fields
	playerNames map[string]*connectedPlayer    // lower case usernames map
	playerIDs   map[uuid.UUID]*connectedPlayer // uuids map

	connectionsQuota *addrquota.Quota
	loginsQuota      *addrquota.Quota
}

// Options are the options for a new Java edition Proxy.
type Options struct {
	// Config requires configuration
	// validated by cfg.Validate.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
}

// New returns a new Proxy ready to start.
func New(options Options) (p *Proxy, err error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}

	p = &Proxy{
		log:         logr.Discard(), // updated by Proxy.Start
		cfg:         options.Config,
		event:       eventMgr,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}

	// Connection & login rate limiters
	p.initQuota(&options.Config.Quota)

	return p, nil
}

func (p *Proxy) initQuota(quota *config.Quota) {
	q := quota.Connections
	if q.Enabled {
		p.connectionsQuota = addrquota.NewQuota(q.OPS, q.Burst, q.MaxEntries)
	} else {
		p.connectionsQuota = nil
	}

	q = quota.Logins
	if q.Enabled {
		p.loginsQuota = addrquota.NewQuota(q.OPS, q.Burst, q.MaxEntries)
	} else {
		p.loginsQuota = nil
	}
}

// ErrProxyAlreadyRun is returned by Proxy.Run if the proxy instance was already run.
var ErrProxyAlreadyRun = errors.New("proxy was already run, create a new one")

// Start runs the Java edition Proxy, blocks until the proxy is
// Shutdown or an error occurred while starting.
// The Proxy is already shutdown on method return.
// Another method of stopping the Proxy is to call Shutdown.
// A Proxy can only be run once or ErrProxyAlreadyRun is returned.
func (p *Proxy) Start(ctx context.Context) error {
	p.closeMu.Lock()
	if p.started {
		p.closeMu.Unlock()
		return ErrProxyAlreadyRun
	}
	p.started = true
	now := time.Now()
	p.startTime.Store(&now)
	p.log = logr.FromContextOrDiscard(ctx)

	p.startCtx, p.cancelStart = context.WithCancel(ctx)
	ctx = p.startCtx
	defer p.cancelStart()
	p.closeMu.Unlock()

	logInfo := func() {
		if p.cfg.Debug {
			p.log.Info("running in debug mode")
		}
		if p.cfg.Lite.Enabled {
			p.log.Info("running in lite mode")
		}
		if p.cfg.ProxyProtocol {
			p.log.Info("proxy protocol enabled")
		}
		if p.cfg.Auth.SessionServerURL != nil {
			p.log.Info("using custom authentication server", "url", p.cfg.Auth.SessionServerURL)
		}
	}
	logInfo()

	defer func() {
		p.Shutdown(p.config().ShutdownReason.T()) // disconnects players
	}()

	eg, ctx := errgroup.WithContext(ctx)
	listen := func(addr string) context.CancelFunc {
		lnCtx, stop := context.WithCancel(ctx)
		eg.Go(func() error {
			defer stop()
			return p.listenAndServe(lnCtx, addr)
		})
		return stop
	}

	stopLn := listen(p.cfg.Bind)

	// Listen for config reloads until we exit
	defer reload.Subscribe(p.event, func(e *javaConfigUpdateEvent) {
		*p.cfg = *e.Config
		p.initQuota(&e.Config.Quota)
		if e.PrevConfig.Bind != e.Config.Bind {
			p.closeMu.Lock()
			stopLn()
			stopLn = listen(e.Config.Bind)
			p.closeMu.Unlock()
		}
		if e.Config.Lite.Enabled {
			// reset whole cache if routes have changed because
			// backend addrs might have moved to another route or a cacheTTL changed
			if func() bool {
				if len(e.Config.Lite.Routes) != len(e.PrevConfig.Lite.Routes) {
					return true
				}
				for i, route := range e.Config.Lite.Routes {
					if !route.Equal(&e.PrevConfig.Lite.Routes[i]) {
						return true
					}
				}
				return false
			}() {
				lite.ResetPingCache()
				p.log.Info("lite ping cache was reset")
			}
		} else {
			lite.ResetPingCache()
		}
		logInfo()
	})()

	return eg.Wait()
}

type javaConfigUpdateEvent = reload.ConfigUpdateEvent[config.Config]

// Shutdown stops the Proxy and/or blocks until the Proxy has finished shutdown.
//
// It first stops listening for new connections, disconnects
// all existing connections with the given reason (nil = blank reason)
// and waits for all event subscribers to finish.
func (p *Proxy) Shutdown(reason component.Component) {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if !p.started {
		return // not started or already shutdown
	}
	p.started = false
	p.cancelStart()

	p.log.Info("shutting down the proxy...")
	shutdownTime := time.Now()
	defer func() {
		p.log.Info("finished shutdown.",
			"shutdownTime", time.Since(shutdownTime).Round(time.Microsecond).String(),
			"totalTime", time.Since(*p.startTime.Load()).Round(time.Millisecond).String())
	}()

	reasonStr := new(strings.Builder)
	if reason != nil && !reflect.ValueOf(reason).IsNil() {
		err := (&legacy.Legacy{}).Marshal(reasonStr, reason)
		if err != nil {
			p.log.Error(err, "error marshal disconnect reason to legacy format")
		}
	}

	p.log.Info("disconnecting all players...", "reason", reasonStr.String())
	disconnectTime := time.Now()
	p.DisconnectAll(reason)
	p.log.Info("disconnected all players.", "time", time.Since(disconnectTime).String())

	p.log.Info("waiting for all event handlers to complete...")
	p.event.Wait()
}

// Config returns the cfg used by the Proxy.
func (p *Proxy) Config() config.Config {
	return *p.cfg
}

func (p *Proxy) config() *config.Config {
	return p.cfg
}

// DisconnectAll disconnects all current connected players
// in parallel and waits until all players have been disconnected.
func (p *Proxy) DisconnectAll(reason component.Component) {
	p.muP.RLock()
	players := p.playerIDs
	p.muP.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(players))
	for _, p := range players {
		go func(p *connectedPlayer) {
			defer wg.Done()
			p.Disconnect(reason)
		}(p)
	}
	wg.Wait()
}

// listenAndServe starts listening for connections on addr until closed channel receives.
func (p *Proxy) listenAndServe(ctx context.Context, addr string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = ln.Close() }()

	defer p.log.Info("stopped listening for new connections", "addr", addr)
	p.log.Info("listening for connections", "addr", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) && errs.IsConnClosedErr(opErr.Err) {
				// Listener was closed
				return nil
			}
			return fmt.Errorf("error accepting new connection: %w", err)
		}

		if p.cfg.ProxyProtocol {
			conn = proxyproto.NewConn(conn)
		}

		go p.HandleConn(conn)
	}
}

// HandleConn handles a just-accepted client connection
// that has not had any I/O performed on it yet.
func (p *Proxy) HandleConn(raw net.Conn) {
	if p.connectionsQuota != nil && p.connectionsQuota.Blocked(netutil.Host(raw.RemoteAddr())) {
		p.log.Info("connection exceeded rate limit, closed", "remoteAddr", raw.RemoteAddr())
		_ = raw.Close()
		return
	}

	// Create context for connection
	ctx, ok := raw.(context.Context)
	if !ok {
		ctx = context.Background()
	}
	ctx = logr.NewContext(ctx, p.log)

	// Create client connection
	conn, readLoop := netmc.NewMinecraftConn(
		ctx, raw, proto.ServerBound,
		time.Duration(p.cfg.ReadTimeout)*time.Millisecond,
		time.Duration(p.cfg.ConnectionTimeout)*time.Millisecond,
		p.cfg.Compression.Level,
	)
	conn.SetActiveSessionHandler(state.Handshake, newHandshakeSessionHandler(conn, &sessionHandlerDeps{
		proxy:          p,
		configProvider: p,
		eventMgr:       p.event,
		loginsQuota:    p.loginsQuota,
	}))
	readLoop()
}

// PlayerCount returns the number of players on the proxy.
func (p *Proxy) PlayerCount() int {
	p.muP.RLock()
	defer p.muP.RUnlock()
	return len(p.playerIDs)
}

// Players returns all players on the proxy.
func (p *Proxy) Players() []Player {
	p.muP.RLock()
	playerIDs := p.playerIDs
	p.muP.RUnlock()
	pls := make([]Player, 0, len(playerIDs))
	for _, player := range playerIDs {
		pls = append(pls, player)
	}
	return pls
}

// Player returns the online player by their Minecraft id.
// Returns nil if the player was not found.
func (p *Proxy) Player(id uuid.UUID) Player {
	p.muP.RLock()
	defer p.muP.RUnlock()
	player, ok := p.playerIDs[id]
	if !ok {
		return nil // return correct nil
	}
	return player
}

// PlayerByName returns the online player by their Minecraft name (search is case-insensitive).
// Returns nil if the player was not found.
func (p *Proxy) PlayerByName(username string) Player {
	player := p.playerByName(username)
	if player == (*connectedPlayer)(nil) {
		return nil // return correct nil
	}
	return player
}
func (p *Proxy) playerByName(username string) *connectedPlayer {
	p.muP.RLock()
	defer p.muP.RUnlock()
	player, ok := p.playerNames[strings.ToLower(username)]
	if !ok {
		return nil
	}
	return player
}

func (p *Proxy) canRegisterConnection(player *connectedPlayer) bool {
	c := p.cfg
	if c.OnlineMode && c.OnlineModeKickExistingPlayers {
		return true
	}
	lowerName := strings.ToLower(player.Username())
	p.muP.RLock()
	defer p.muP.RUnlock()
	return p.playerNames[lowerName] == nil && p.playerIDs[player.ID()] == nil
}

// Attempts to register the connection with the proxy.
func (p *Proxy) registerConnection(player *connectedPlayer) bool {
	lowerName := strings.ToLower(player.Username())
	c := p.cfg

retry:
	p.muP.Lock()
	if c.OnlineModeKickExistingPlayers {
		existing, ok := p.playerIDs[player.ID()]
		if ok {
			// Make sure we disconnect existing duplicate
			// player connection before we register the new one.
			//
			// Disconnecting the existing connection will call p.unregisterConnection in the
			// teardown needing the p.muP.Lock() so we unlock.
			p.muP.Unlock()
			existing.disconnectDueToDuplicateConnection.Store(true)
			existing.Disconnect(&component.Translation{
				Key: "multiplayer.disconnect.duplicate_login",
			})
			// Now we can retry in case another duplicate connection
			// occurred before we could acquire the lock at `retry`.
			//
			// Meaning we keep disconnecting incoming duplicates until
			// we can register our connection, but this shall be uncommon anyway. :)
			goto retry
		}
	} else {
		_, exists := p.playerNames[lowerName]
		if exists {
			return false
		}
		_, exists = p.playerIDs[player.ID()]
		if exists {
			return false
		}
	}

	p.playerIDs[player.ID()] = player
	p.playerNames[lowerName] = player
	p.muP.Unlock()
	return true
}

// unregisters a connected player
func (p *Proxy) unregisterConnection(player *connectedPlayer) (found bool) {
	p.muP.Lock()
	defer p.muP.Unlock()
	_, found = p.playerIDs[player.ID()]
	delete(p.playerNames, strings.ToLower(player.Username()))
	delete(p.playerIDs, player.ID())
	return found
}

//
//
//
//
//

type (
	configProvider interface {
		config() *config.Config
	}
)
