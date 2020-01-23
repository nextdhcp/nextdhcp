package memory

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/ppacher/webthings-mqtt-gateway/pkg/mutex"
)

type entry struct {
	ip         net.IP
	clientID   string
	leased     bool
	expiration time.Time
}

func (e *entry) key() key {
	return key(fmt.Sprintf("%s:%s", e.clientID, e.ip.String()))
}

type key string

// Storage implements the storage.LeaseStorage interface but
// does not provide any persistence at all as every IP lease
// is only kept in memory
type Storage struct {
	l         *mutex.Mutex // context.Context aware mutex to protect all fields below
	entries   map[key]*entry
	ips       map[string]key
	clientIDs map[string]key
}

// New returns a new memory storage
func New() *Storage {
	return makeStorage()
}

func makeStorage() *Storage {
	return &Storage{
		l:         mutex.New(),
		entries:   make(map[key]*entry),
		ips:       make(map[string]key),
		clientIDs: make(map[string]key),
	}
}

// Create implements storage.LeaseStorage
func (s *Storage) Create(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	if !s.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer s.l.Unlock()

	e := &entry{
		ip:         ip,
		clientID:   clientID,
		leased:     leased,
		expiration: expiration,
	}

	if key, ok := s.ips[e.ip.String()]; ok {
		return &storage.ErrDuplicateIP{
			IP:       ip,
			ClientID: s.entries[key].clientID,
		}
	}

	if key, ok := s.clientIDs[e.clientID]; ok {
		return &storage.ErrDuplicateClientID{
			ClientID: clientID,
			IP:       s.entries[key].ip,
		}
	}

	newKey := e.key()
	if _, ok := s.entries[newKey]; ok {
		return storage.ErrAlreadyCreated
	}

	s.entries[newKey] = e
	s.ips[ip.String()] = newKey
	s.clientIDs[clientID] = newKey

	return nil
}

// Delete implements storage.LeaseStorage
func (s *Storage) Delete(ctx context.Context, ip net.IP, clientID string) error {
	if !s.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer s.l.Unlock()

	entryKey, ok := s.ips[ip.String()]
	if !ok {
		return &storage.ErrIPNotFound{IP: ip}
	}

	entry, ok := s.entries[entryKey]
	if !ok {
		return errors.New("internal error: database inconsistency")
	}

	if clientID != "" && clientID != entry.clientID {
		return storage.ErrClientMismatch
	}

	delete(s.entries, entryKey)
	delete(s.ips, entry.ip.String())
	delete(s.clientIDs, entry.clientID)

	return nil
}

// Update implements storage.LeaseStorage
func (s *Storage) Update(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	if !s.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer s.l.Unlock()

	e := &entry{
		ip:         ip,
		clientID:   clientID,
		leased:     leased,
		expiration: expiration,
	}
	entryKey := e.key()

	if _, ok := s.entries[entryKey]; !ok {
		// TODO(ppacher): there should be a better error
		return &storage.ErrIPNotFound{IP: ip}
	}

	s.entries[entryKey] = e

	return nil
}

// FindByIP implements storage.LeaseStorage
func (s *Storage) FindByIP(ctx context.Context, ip net.IP) (string, bool, time.Time, error) {
	if !s.l.TryLock(ctx) {
		return "", false, time.Time{}, ctx.Err()
	}
	defer s.l.Unlock()

	entryKey, ok := s.ips[ip.String()]
	if !ok {
		return "", false, time.Time{}, &storage.ErrIPNotFound{IP: ip}
	}

	e, ok := s.entries[entryKey]
	if !ok {
		return "", false, time.Time{}, errors.New("internal error: database inconsistency")
	}

	return e.clientID, e.leased, e.expiration, nil
}

// FindByID implements storage.LeaseStorage
func (s *Storage) FindByID(ctx context.Context, clientID string) (net.IP, bool, time.Time, error) {
	if !s.l.TryLock(ctx) {
		return nil, false, time.Time{}, ctx.Err()
	}
	defer s.l.Unlock()

	key, ok := s.clientIDs[clientID]
	if !ok {
		// TODO(ppacher) better error
		return nil, false, time.Time{}, &storage.ErrIPNotFound{}
	}

	e, ok := s.entries[key]
	if !ok {
		return nil, false, time.Time{}, errors.New("internal error: database inconsistency")
	}

	return e.ip, e.leased, e.expiration, nil
}

// ListIPs implements storage.LeaseStorage
func (s *Storage) ListIPs(ctx context.Context) ([]net.IP, error) {
	if !s.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer s.l.Unlock()

	ips := make([]net.IP, 0, len(s.ips))
	for ip := range s.ips {
		ips = append(ips, net.ParseIP(ip))
	}

	return ips, nil
}

// ListIDs implements storage.LeaseStorage
func (s *Storage) ListIDs(ctx context.Context) ([]string, error) {
	if !s.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer s.l.Unlock()

	ids := make([]string, 0, len(s.clientIDs))
	for id := range s.clientIDs {
		ids = append(ids, id)
	}

	return ids, nil
}

// compile time check
var _ storage.LeaseStorage = &Storage{}
