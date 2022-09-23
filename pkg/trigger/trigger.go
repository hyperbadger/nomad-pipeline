package trigger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/fsnotify/fsnotify"
	nomad "github.com/hashicorp/nomad/api"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

func containsStr(match string, strs []string) bool {
	for _, str := range strs {
		if str == match {
			return true
		}
	}
	return false
}

type Dispatch struct {
	Meta    map[string]string `yaml:"meta" json:"meta"`
	Payload string            `yaml:"payload" json:"payload"`
}

type Triggerer interface {
	Init(context.Context) error
	Key() string
	Run(context.Context, func(Dispatch) error, chan<- error)
	Shutdown(context.Context) error
}

type TriggerCommon struct {
	JobID string `yaml:"job_id"`
	Type  string `yaml:"type"`
}

type Trigger struct {
	TriggerCommon
	Trigger Triggerer
	key     string
	stop    chan struct{}
	logger  *zap.SugaredLogger
}

func (t *Trigger) UnmarshalYAML(unmarshal func(interface{}) error) error {
	tc := TriggerCommon{}
	err := unmarshal(&tc)
	if err != nil {
		return err
	}

	t.TriggerCommon = tc
	switch tc.Type {
	case "simplepubsub":
		var sps struct {
			Trigger SimplePubSubTrigger `yaml:"trigger"`
		}
		err = unmarshal(&sps)
		if err != nil {
			return err
		}
		t.Trigger = &sps.Trigger
	case "s3":
		var s3t struct {
			Trigger S3Trigger `yaml:"trigger"`
		}
		err = unmarshal(&s3t)
		if err != nil {
			return err
		}
		t.Trigger = &s3t.Trigger
	}
	return nil
}

func (t *Trigger) Init(ctx context.Context, logger *zap.SugaredLogger) error {
	t.logger = logger.With("job_id", t.JobID, "type", t.Type, "key", t.Key())
	return t.Trigger.Init(ctx)
}

func (t *Trigger) Key() string {
	if len(t.key) > 0 {
		return t.key
	}
	k := t.JobID + "-" + t.Type + t.Trigger.Key()
	h := sha256.New()
	h.Write([]byte(k))
	s := h.Sum(nil)
	t.key = hex.EncodeToString(s)
	return t.key
}

func (t *Trigger) Run(ctx context.Context, jobsApi *nomad.Jobs) {
	errCh := make(chan error)
	doneCh := make(chan struct{})

	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()

	f := func(d Dispatch) error {
		op := func() error {
			resp, wm, err := jobsApi.Dispatch(t.JobID, d.Meta, []byte(d.Payload), &nomad.WriteOptions{})
			if err != nil {
				return err
			}
			t.logger.Infow("dispatched job", "dispatched_job_id", resp.DispatchedJobID, "dur_ms", wm.RequestTime.Milliseconds())
			return nil
		}

		log := func(err error, dur time.Duration) {
			t.logger.Errorw("error dispatching job, retrying in "+dur.String(), "error", err)
		}

		err := backoff.RetryNotify(op, backoff.WithContext(backoff.NewExponentialBackOff(), rctx), log)
		if err != nil {
			return fmt.Errorf("error dispatching job %w", err)
		}

		return nil
	}

	go func() {
		t.Trigger.Run(rctx, f, errCh)
		close(doneCh)
	}()

handler:
	for {
		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) {
				t.logger.Errorw("error from running trigger", "error", err)
			}
		case <-t.stop:
			rcancel()
		case <-doneCh:
			// TODO: shutdown should have a different context than the main one
			err := t.Trigger.Shutdown(context.TODO())
			if err != nil {
				t.logger.Warnw("error shutting down trigger, continuing to remove from manager", "error", err)
			}
			break handler
		}
	}
}

type TriggerManager struct {
	triggers map[string]*Trigger
	path     string
	jobsApi  *nomad.Jobs
	logger   *zap.SugaredLogger
	wg       sync.WaitGroup
}

func NewTriggerManager(ctx context.Context, tPath string, nClient *nomad.Client, logger *zap.SugaredLogger) (*TriggerManager, error) {
	tm := TriggerManager{
		triggers: make(map[string]*Trigger),
		path:     tPath,
		jobsApi:  nClient.Jobs(),
		logger:   logger,
	}

	return &tm, nil
}

func (tm *TriggerManager) AddTrigger(ctx context.Context, key string, t *Trigger) {
	logger := tm.logger.With("job_id", t.JobID, "type", t.Type, "key", key)
	logger.Info("adding trigger")

	err := t.Init(ctx, tm.logger)
	if err != nil {
		logger.Errorw("error adding trigger", "error", err)
		return
	}

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		t.Run(ctx, tm.jobsApi)
	}()

	tm.triggers[key] = t
}

func (tm *TriggerManager) RemoveTrigger(ctx context.Context, key string) {
	t := tm.triggers[key]
	tm.logger.Infow("removing trigger", "job_id", t.JobID, "type", t.Type, "key", key)
	t.stop <- struct{}{}
	delete(tm.triggers, key)
}

func (tm *TriggerManager) UpdateTriggers(ctx context.Context) error {
	tBytes, err := os.ReadFile(tm.path)
	if err != nil {
		return fmt.Errorf("error loading triggers (path: %v): %w", tm.path, err)
	}

	ts := []Trigger{}
	err = yaml.Unmarshal(tBytes, &ts)
	if err != nil {
		return fmt.Errorf("error reading triggers yaml: %w", err)
	}

	tsm := make(map[string]*Trigger)
	for _, t := range ts {
		tsm[t.Key()] = &t
	}

	ets := maps.Keys(tm.triggers)
	nts := maps.Keys(tsm)

	ats := make([]string, 0)
	for _, t := range nts {
		if !containsStr(t, ets) {
			ats = append(ats, t)
		}
	}

	rts := make([]string, 0)
	for _, t := range ets {
		if !containsStr(t, nts) {
			rts = append(rts, t)
		}
	}

	for _, t := range ats {
		tm.AddTrigger(ctx, t, tsm[t])
	}

	for _, t := range rts {
		tm.RemoveTrigger(ctx, t)
	}

	return nil
}

func (tm *TriggerManager) ListenAndDispatch(ctx context.Context) error {
	err := tm.UpdateTriggers(ctx)
	if errors.Is(err, os.ErrNotExist) {
		tm.logger.Warnw("triggers file doesn't exist, trigger manager will be disabled", "path", tm.path)
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error initializing file watch for triggers: %w", err)
	}
	defer watcher.Close()

	err = watcher.Add(tm.path)
	if err != nil {
		return fmt.Errorf("error adding trigger file for watching: %w", err)
	}

handler:
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) {
				err = tm.UpdateTriggers(ctx)
				tm.logger.Warnw("error updating triggers from file", "error", err)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			tm.logger.Warnw("error watching triggers", "error", err)
		case <-ctx.Done():
			break handler
		}
	}

	tm.wg.Wait()

	return nil
}
