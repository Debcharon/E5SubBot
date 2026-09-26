package renewal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
)

var ErrAlreadyRunning = errors.New("renewal task is already running")

type Store interface {
	ListAll(context.Context) ([]account.Client, error)
	Update(context.Context, *account.Client) error
	DeleteByID(context.Context, int) error
}

type Microsoft interface {
	GetOutlookMails(context.Context, string, string, string) (string, error)
}

type Settings interface {
	Current() config.Config
}

type Result struct {
	Client  account.Client
	Err     error
	Removed bool
}

type Report struct {
	Results  []Result
	Started  time.Time
	Duration time.Duration
}

type Runner struct {
	store      Store
	microsoft  Microsoft
	settings   Settings
	mu         sync.Mutex
	errorTimes map[int]int
}

func New(store Store, microsoft Microsoft, settings Settings) *Runner {
	return &Runner{store: store, microsoft: microsoft, settings: settings, errorTimes: make(map[int]int)}
}

func (r *Runner) Run(ctx context.Context) (Report, error) {
	if !r.mu.TryLock() {
		return Report{}, ErrAlreadyRunning
	}
	defer r.mu.Unlock()

	report := Report{Started: time.Now()}
	clients, err := r.store.ListAll(ctx)
	if err != nil {
		return report, fmt.Errorf("list accounts: %w", err)
	}
	report.Results = make([]Result, len(clients))
	if len(clients) == 0 {
		report.Duration = time.Since(report.Started)
		return report, nil
	}
	workers := r.settings.Current().Workers
	if workers > len(clients) {
		workers = len(clients)
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				client := clients[index]
				refresh, err := r.microsoft.GetOutlookMails(ctx, client.ClientID, client.ClientSecret, client.RefreshToken)
				if err == nil {
					client.RefreshToken = refresh
				}
				report.Results[index] = Result{Client: client, Err: err}
			}
		}()
	}
	for index := range clients {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return report, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return report, err
	}

	limit := r.settings.Current().ErrorLimit
	for index := range report.Results {
		result := &report.Results[index]
		if result.Err == nil {
			r.errorTimes[result.Client.ID] = 0
			if err := r.store.Update(ctx, &result.Client); err != nil {
				result.Err = fmt.Errorf("save refreshed token: %w", err)
			}
			continue
		}
		r.errorTimes[result.Client.ID]++
		if r.errorTimes[result.Client.ID] <= limit {
			continue
		}
		if err := r.store.DeleteByID(ctx, result.Client.ID); err != nil {
			result.Err = fmt.Errorf("%w; auto unbind: %v", result.Err, err)
			continue
		}
		result.Removed = true
		delete(r.errorTimes, result.Client.ID)
	}
	report.Duration = time.Since(report.Started)
	return report, nil
}
