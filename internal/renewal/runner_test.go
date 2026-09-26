package renewal

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
)

type testSettings struct{ cfg config.Config }

func (s testSettings) Current() config.Config { return s.cfg }

type testStore struct {
	mu      sync.Mutex
	clients []account.Client
	deleted []int
}

func (s *testStore) ListAll(context.Context) ([]account.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]account.Client(nil), s.clients...), nil
}

func (s *testStore) Update(_ context.Context, client *account.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.clients {
		if s.clients[i].ID == client.ID {
			s.clients[i] = *client
		}
	}
	return nil
}

func (s *testStore) DeleteByID(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, id)
	for i, client := range s.clients {
		if client.ID == id {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			break
		}
	}
	return nil
}

type testMicrosoft struct {
	block   chan struct{}
	entered chan struct{}
}

func (m testMicrosoft) GetOutlookMails(_ context.Context, id, _, _ string) (string, error) {
	if m.entered != nil {
		m.entered <- struct{}{}
		<-m.block
	}
	if id == "broken" {
		return "", errors.New("Graph unavailable")
	}
	return "rotated", nil
}

func TestRunPersistsRefreshAndLimitsAutoUnbind(t *testing.T) {
	store := &testStore{clients: []account.Client{
		{ID: 1, ClientID: "working", RefreshToken: "old"},
		{ID: 2, ClientID: "broken", RefreshToken: "old"},
	}}
	runner := New(store, testMicrosoft{}, testSettings{config.Config{Workers: 2, ErrorLimit: 1}})
	first, err := runner.Run(context.Background())
	if err != nil || len(first.Results) != 2 || first.Results[1].Removed {
		t.Fatalf("unexpected first run: report=%+v err=%v", first, err)
	}
	if store.clients[0].RefreshToken != "rotated" || len(store.deleted) != 0 {
		t.Fatalf("token or error threshold incorrect: clients=%+v deleted=%v", store.clients, store.deleted)
	}
	second, err := runner.Run(context.Background())
	if err != nil || !second.Results[1].Removed || len(store.deleted) != 1 || store.deleted[0] != 2 {
		t.Fatalf("repeated failure did not remove only broken account: report=%+v deleted=%v err=%v", second, store.deleted, err)
	}
}

func TestRunDoesNotOverlap(t *testing.T) {
	entered := make(chan struct{}, 1)
	block := make(chan struct{})
	store := &testStore{clients: []account.Client{{ID: 1, ClientID: "working"}}}
	runner := New(store, testMicrosoft{block: block, entered: entered}, testSettings{config.Config{Workers: 1, ErrorLimit: 1}})
	finished := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background())
		finished <- err
	}()
	<-entered
	if _, err := runner.Run(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("overlapping task was not rejected: %v", err)
	}
	close(block)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
