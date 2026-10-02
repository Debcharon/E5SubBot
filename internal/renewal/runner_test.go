package renewal

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
)

type testSettings struct{ cfg config.Config }

func (s testSettings) Current() config.Config { return s.cfg }

type testStore struct {
	mu        sync.Mutex
	clients   []account.Client
	deleted   []int
	updateErr error
}

func (s *testStore) ListAll(context.Context) ([]account.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]account.Client(nil), s.clients...), nil
}

func (s *testStore) Update(_ context.Context, client *account.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updateErr != nil {
		return s.updateErr
	}
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
	if id == "unauthorized" {
		return "", &microsoft.APIError{Status: 400, Code: "invalid_grant"}
	}
	if id == "rotated-failed" {
		return "rotated", errors.New("Graph unavailable")
	}
	return "rotated", nil
}

func TestRunRetainsAccountsAndPersistsRefreshAfterGraphFailure(t *testing.T) {
	store := &testStore{clients: []account.Client{
		{ID: 1, ClientID: "working", RefreshToken: "old"},
		{ID: 2, ClientID: "broken", RefreshToken: "old"},
		{ID: 3, ClientID: "unauthorized", RefreshToken: "old"},
		{ID: 4, ClientID: "rotated-failed", RefreshToken: "old", UpdatedAtUnix: 123},
	}}
	runner := New(store, testMicrosoft{}, testSettings{config.Config{Workers: 2, ErrorLimit: 1}})
	first, err := runner.Run(context.Background())
	if err != nil || len(first.Results) != 4 || first.Results[2].NeedsAuthorization {
		t.Fatalf("unexpected first run: report=%+v err=%v", first, err)
	}
	if store.clients[0].RefreshToken != "rotated" || len(store.deleted) != 0 {
		t.Fatalf("token or error threshold incorrect: clients=%+v deleted=%v", store.clients, store.deleted)
	}
	second, err := runner.Run(context.Background())
	if err != nil || second.Results[1].NeedsAuthorization || !second.Results[2].NeedsAuthorization || len(store.deleted) != 0 || len(store.clients) != 4 {
		t.Fatalf("accounts must survive both temporary and authorization failures: report=%+v deleted=%v err=%v", second, store.deleted, err)
	}
	if store.clients[3].RefreshToken != "rotated" || store.clients[3].UpdatedAtUnix != 123 {
		t.Fatalf("rotation must preserve last successful renewal time: %+v", store.clients[3])
	}
	status := runner.Status()
	if status.Running || status.Success != 1 || status.Failed != 3 || status.Finished.IsZero() {
		t.Fatalf("incorrect task status: %+v", status)
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
	if !runner.Status().Running {
		t.Fatal("status did not report active task")
	}
	if _, err := runner.Run(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("overlapping task was not rejected: %v", err)
	}
	close(block)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestTokenSaveFailureIsReportedAsFailure(t *testing.T) {
	store := &testStore{clients: []account.Client{{ID: 1, ClientID: "working", RefreshToken: "old"}}, updateErr: errors.New("database unavailable")}
	runner := New(store, testMicrosoft{}, testSettings{config.Config{Workers: 1, ErrorLimit: 0}})
	report, err := runner.Run(context.Background())
	if err != nil || report.Results[0].Err == nil || report.Results[0].NeedsAuthorization || store.clients[0].RefreshToken != "old" || runner.Status().Failed != 1 {
		t.Fatalf("save failure was hidden or treated as authorization failure: report=%+v err=%v", report, err)
	}
}
