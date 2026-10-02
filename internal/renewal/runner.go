package renewal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
)

var ErrAlreadyRunning = errors.New("renewal task is already running")

type Store interface {
	ListAll(context.Context) ([]account.Client, error)
	Update(context.Context, *account.Client) error
}

type Microsoft interface {
	GetOutlookMails(context.Context, string, string, string) (string, error)
}

type Settings interface {
	Current() config.Config
}

type Result struct {
	Client             account.Client
	Err                error
	NeedsAuthorization bool
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
	statusMu   sync.RWMutex
	status     Status
}

type Status struct {
	Running  bool
	Started  time.Time
	Finished time.Time
	Success  int
	Failed   int
	Error    string
}

func (r *Runner) Status() Status {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

func New(store Store, microsoft Microsoft, settings Settings) *Runner {
	return &Runner{store: store, microsoft: microsoft, settings: settings, errorTimes: make(map[int]int)}
}

func (r *Runner) Run(ctx context.Context) (report Report, runErr error) {
	if !r.mu.TryLock() {
		return Report{}, ErrAlreadyRunning
	}
	defer r.mu.Unlock()

	report = Report{Started: time.Now()}
	r.statusMu.Lock()
	r.status = Status{Running: true, Started: report.Started}
	r.statusMu.Unlock()
	defer func() {
		r.statusMu.Lock()
		defer r.statusMu.Unlock()
		r.status.Running = false
		r.status.Finished = time.Now()
		if runErr != nil {
			r.status.Error = runErr.Error()
			return
		}
		for _, result := range report.Results {
			if result.Err == nil {
				r.status.Success++
			} else {
				r.status.Failed++
			}
		}
	}()
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
				if refresh != "" {
					client.RefreshToken = refresh
				}
				if err == nil {
					client.UpdatedAtUnix = time.Now().Unix()
				}
				if refresh != "" {
					if saveErr := r.store.Update(ctx, &client); saveErr != nil {
						err = errors.Join(err, fmt.Errorf("save refreshed token: %w", saveErr))
					}
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
			delete(r.errorTimes, result.Client.ID)
			continue
		}
		if !microsoft.RequiresAuthorization(result.Err) {
			delete(r.errorTimes, result.Client.ID)
			continue
		}
		r.errorTimes[result.Client.ID]++
		if r.errorTimes[result.Client.ID] <= limit {
			continue
		}
		result.NeedsAuthorization = true
	}
	report.Duration = time.Since(report.Started)
	return report, nil
}
