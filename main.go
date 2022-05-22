package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	nomad "github.com/hashicorp/nomad/api"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

const (
	rootTag            = "nomad-pipeline/root"
	nextTag            = "nomad-pipeline/next"
	dependenciesTag    = "nomad-pipeline/dependencies"
	countTag           = "nomad-pipeline/count"
	dynamicTasksTag    = "nomad-pipeline/dynamic-tasks"
	dynamicMemoryMBTag = "nomad-pipeline/dynamic-memory-mb"

	// internal tags, not  meant to be set by user
	_parentTask = "nomad-pipeline/_parent-task"
)

func init() {
	if len(os.Getenv("NOMAD_PIPELINE_LOG_JSON")) > 0 {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if len(os.Getenv("NOMAD_PIPELINE_DEBUG")) > 0 {
		log.SetLevel(log.DebugLevel)
	}
}

func i2p(i int) *int {
	return &i
}

func dedupStr(dup []string) []string {
	seen := make(map[string]bool)
	dedup := make([]string, 0)

	for _, item := range dup {
		if _, ok := seen[item]; !ok {
			seen[item] = true
			dedup = append(dedup, item)
		}
	}

	return dedup
}

func dedupAllocs(dup []*nomad.AllocationListStub) []*nomad.AllocationListStub {
	seen := make(map[string]bool)
	dedup := make([]*nomad.AllocationListStub, 0)

	for _, item := range dup {
		if _, ok := seen[item.Name]; !ok {
			seen[item.Name] = true
			dedup = append(dedup, item)
		}
	}

	return dedup
}

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}

func copyMapInterface(og map[string]interface{}) map[string]interface{} {
	nm := make(map[string]interface{})
	for k, v := range og {
		nm[k] = v
	}
	return nm
}

func copyMapString(og map[string]string) map[string]string {
	nm := make(map[string]string)
	for k, v := range og {
		nm[k] = v
	}
	return nm
}

func clearECh(ch <-chan *nomad.Events) {
	for len(ch) > 0 {
		<-ch
	}
}

func lookupTask(tg *nomad.TaskGroup, tName string) *nomad.Task {
	for _, t := range tg.Tasks {
		if t.Name == tName {
			return t
		}
	}

	return nil
}

func split(toSplit string) []string {
	parts := strings.Split(toSplit, ",")
	tParts := make([]string, len(parts))
	for i, part := range parts {
		tPart := strings.TrimSpace(part)
		tParts[i] = tPart
	}
	return tParts
}

func tgAllocated(allocs []*nomad.AllocationListStub, groups []string) (allocated bool) {
	for _, alloc := range allocs {
		for _, group := range groups {
			if alloc.TaskGroup == group {
				allocated = true
			}
		}
	}

	return
}

func successState(state *nomad.TaskState) bool {
	if state.Failed {
		return false
	}

	tEvents := make([]*nomad.TaskEvent, 0)
	for _, event := range state.Events {
		if event.Type == nomad.TaskTerminated {
			tEvents = append(tEvents, event)
		}
	}

	if len(tEvents) == 0 {
		log.Warnf("job not marked as failed and no events")
		return false
	}

	// sort by time to get latest "Terminated" event
	sort.Slice(tEvents, func(i, j int) bool { return tEvents[i].Time < tEvents[j].Time })

	// check exit code of the latest "Terminated"
	if codeStr, ok := tEvents[len(tEvents)-1].Details["exit_code"]; ok {
		code, err := strconv.Atoi(codeStr)
		if err != nil {
			log.Errorf("error converting exit code (%v) to integer: %v", codeStr, err)
			return false
		}

		if code > 0 {
			log.Warnf("exit code (%v) and task not marked as failed, likely the job was stopped by signal", code)
		}

		return code == 0
	}

	return true
}

func tgDone(allocs []*nomad.AllocationListStub, groups []string, success bool) bool {
	if len(groups) == 0 || len(allocs) == 0 {
		return false
	}

	// sort allocations from newest to oldest job version
	sort.Slice(allocs, func(i, j int) bool { return allocs[i].JobVersion > allocs[j].JobVersion })
	// deduping will use the latest job version and remove older ones
	// hence the prior sorting
	allocs = dedupAllocs(allocs)

	// keeps track of how many allocations a task group expects to be complete
	groupCount := make(map[string]int, 0)
	for _, alloc := range allocs {
		groupCount[alloc.TaskGroup] += 1
	}

	dGroupCount := make(map[string]int, 0)
	for _, alloc := range allocs {
		for _, group := range groups {
			if alloc.TaskGroup == group {
				tasks := 0
				dTasks := 0
				for task, state := range alloc.TaskStates {
					if task != "wait" && task != "next" {
						tasks++

						if state.State == "dead" && !state.FinishedAt.IsZero() {
							if success && successState(state) {
								dTasks++
							} else if !success {
								dTasks++
							}
						}
					}
				}

				if tasks == dTasks {
					dGroupCount[group] += 1
				}
			}
		}
	}

	dGroups := make([]string, 0)
	for group, count := range dGroupCount {
		log.Debugf("%v => %v completions", group, count)

		if count >= groupCount[group] {
			dGroups = append(dGroups, group)
		}
	}

	if len(dGroups) == 0 {
		return false
	}

	sort.Strings(groups)
	sort.Strings(dGroups)

	dGroups = dedupStr(dGroups)

	return equalStr(groups, dGroups)
}

func loadConfig(cPath string) *Config {
	cBytes, err := os.ReadFile(cPath)

	if errors.Is(err, os.ErrNotExist) {
		log.Warnf("config file doesn't exist (path: %v)", cPath)
		return nil
	}

	if err != nil {
		log.Warnf("error loading config (path: %v): %v", cPath, err)
		return nil
	}

	c := Config{}
	err = yaml.Unmarshal(cBytes, &c)
	if err != nil {
		log.Fatalf("error reading config yaml: %v", err)
	}

	return &c
}

type Task struct {
	Name         string
	Next         []string
	Dependencies []string
}

type Tasks []Task

func (ts Tasks) LookupTask(name string) *Task {
	for _, t := range ts {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

type TaskGroups []nomad.TaskGroup

type Config struct {
}

type PipelineController struct {
	JobID     string
	GroupName string
	TaskName  string
	AllocID   string
	Job       *nomad.Job
	Nomad     *nomad.Client
	JobsAPI   *nomad.Jobs
	AllocsAPI *nomad.Allocations
	Config    *Config
	Image     string
}

func NewPipelineController(cPath string) *PipelineController {
	dc := PipelineController{
		JobID:     os.Getenv("NOMAD_JOB_ID"),
		GroupName: os.Getenv("NOMAD_GROUP_NAME"),
		TaskName:  os.Getenv("NOMAD_TASK_NAME"),
		AllocID:   os.Getenv("NOMAD_ALLOC_ID"),
		Config:    loadConfig(cPath),
	}

	nClient, err := nomad.NewClient(&nomad.Config{
		Address: os.Getenv("NOMAD_ADDR"),
	})
	if err != nil {
		log.Fatalf("error creating client: %v", err)
	}

	dc.Nomad = nClient
	dc.JobsAPI = nClient.Jobs()
	dc.AllocsAPI = nClient.Allocations()

	log.Infof("getting job: %q", dc.JobID)
	job, _, err := dc.JobsAPI.Info(dc.JobID, &nomad.QueryOptions{})
	if err != nil {
		log.Fatalf("error getting job: %v", err)
	}

	dc.Job = job

	return &dc
}

func (pc *PipelineController) UpdateJob() error {
	log.Debugf("updating job with job modify index: %v", *pc.Job.JobModifyIndex)
	r, _, err := pc.JobsAPI.RegisterOpts(
		pc.Job,
		&nomad.RegisterOptions{
			EnforceIndex: true,
			ModifyIndex:  *pc.Job.JobModifyIndex,
		},
		&nomad.WriteOptions{},
	)
	if err != nil {
		return err
	}

	log.Debugf("updated job, new job modify index: %v", r.JobModifyIndex)
	pc.Job.JobModifyIndex = &r.JobModifyIndex

	return nil
}

func (pc *PipelineController) ProcessTaskGroups(filters ...map[string]string) ([]string, error) {
	filter := make(map[string]string)
	for _, _filter := range filters {
		for k, v := range _filter {
			filter[k] = v
		}
	}

	rTasks := make([]string, 0)
	tasks := make(Tasks, 0, len(pc.Job.TaskGroups))

	procTG := pc.Job.LookupTaskGroup(pc.GroupName)
	procTask := lookupTask(procTG, pc.TaskName)

	for _, tGroup := range pc.Job.TaskGroups {
		// skip init group
		if *tGroup.Name == pc.GroupName {
			continue
		}

		for k, v := range filter {
			if tag, ok := tGroup.Meta[k]; ok {
				if tag == v {
					continue
				}
			}
		}

		task := Task{
			Name: *tGroup.Name,
		}

		if next, ok := tGroup.Meta[nextTag]; ok {
			task.Next = split(next)
		}

		if dependencies, ok := tGroup.Meta[dependenciesTag]; ok {
			task.Dependencies = split(dependencies)
		}

		tasks = append(tasks, task)

		if root, ok := tGroup.Meta[rootTag]; ok {
			isRoot, err := strconv.ParseBool(root)
			if err != nil {
				return nil, fmt.Errorf("root meta tag (%v) is set on task group (%v) but is not convertable to a bool", rootTag, tGroup.Name)
			}

			if isRoot {
				rTasks = append(rTasks, *tGroup.Name)
			}
		}

		// not sure if this should be here
		for _, t := range tGroup.Tasks {
			if mem, ok := t.Meta[dynamicMemoryMBTag]; ok {
				meme := os.ExpandEnv(mem)
				memi, err := strconv.Atoi(meme)
				if err != nil {
					return nil, fmt.Errorf("can't convert dynamic memory tag (%v) of value (%v) to a integer", dynamicMemoryMBTag, mem)
				}
				t.Resources.MemoryMB = i2p(memi)
				log.Debugf("setting dynamic memory for task (%v) in task group (%v) to (%v)", t.Name, *tGroup.Name, memi)
			}
		}
	}

	for _, task := range tasks {
		tGroup := pc.Job.LookupTaskGroup(task.Name)
		if tGroup == nil {
			return nil, fmt.Errorf("task not found in job: %v", task.Name)
		}

		for _, nTask := range task.Next {
			if nTGroup := pc.Job.LookupTaskGroup(nTask); nTGroup == nil {
				return nil, fmt.Errorf("next task specified in task (%v) not found in job: %v", task.Name, nTask)
			}
		}

		for _, dTask := range task.Dependencies {
			if dTGroup := pc.Job.LookupTaskGroup(dTask); dTGroup == nil {
				return nil, fmt.Errorf("dependent task specified in task (%v) not found in job: %v", task.Name, dTask)
			}
		}

		if *tGroup.Count > 0 {
			return nil, fmt.Errorf("dag controlled task must have a zero count: %v", task.Name)
		}

		env := copyMapString(procTask.Env)

		dTask := nomad.NewTask("wait", "docker")

		dTask.Lifecycle = &nomad.TaskLifecycle{
			Hook: nomad.TaskLifecycleHookPrestart,
		}

		dTaskCfg := copyMapInterface(procTask.Config)
		dTaskCfg["args"] = append([]string{"-wait"}, task.Dependencies...)
		dTask.Config = dTaskCfg

		dTask.Env = env

		if len(task.Dependencies) > 0 {
			tGroup.AddTask(dTask)
		}

		nTask := nomad.NewTask("next", "docker")

		nTask.Lifecycle = &nomad.TaskLifecycle{
			Hook: nomad.TaskLifecycleHookPoststop,
		}

		nArgs := append([]string{"-next"}, task.Next...)

		if dynTasks, ok := tGroup.Meta[dynamicTasksTag]; ok {
			nArgs = append([]string{"-dynamic-tasks", dynTasks}, nArgs...)
		}

		nTaskCfg := copyMapInterface(procTask.Config)
		nTaskCfg["args"] = nArgs
		nTask.Config = nTaskCfg

		nTask.Env = env

		tGroup.AddTask(nTask)
	}

	return rTasks, nil
}

func (pc *PipelineController) Init() {
	rTasks, err := pc.ProcessTaskGroups()
	if err != nil {
		log.Fatalf("error processing task groups: %v", err)
	}

	err = pc.UpdateJob()
	if err != nil {
		log.Fatalf("error updating job: %v", err)
	}

	if len(rTasks) == 0 {
		log.Fatalf("couldn't find a root task group, need to set the root meta tag (%v)", rootTag)
	}

	pc.Next(rTasks, "")
}

func (pc *PipelineController) Wait(groups []string) {
	jAllocs, meta, err := pc.JobsAPI.Allocations(pc.JobID, true, &nomad.QueryOptions{})
	if err != nil {
		log.Fatalf("error getting job allocations: %v", err)
	}

	if tgDone(jAllocs, groups, true) {
		log.Info("all dependent task groups finished successfully")
		return
	}

	allocStubStore := make(map[string]*nomad.AllocationListStub)

	// initialized alloc store with current state
	for _, alloc := range jAllocs {
		allocStubStore[alloc.ID] = alloc
	}

	eClient := pc.Nomad.EventStream()

	topics := map[nomad.Topic][]string{
		nomad.TopicAllocation: {pc.JobID},
	}

	idx := meta.LastIndex
	log.Debug("event start index: %v", idx)

	eCh := make(<-chan *nomad.Events, 10)
	sub := make(chan bool, 1)
	cancel := func() {}

	// initial subscription
	sub <- true

	eErrs := 0
	for {
		select {
		case <-sub:
			log.Debug("subscribing to event stream")
			ctx := context.Background()
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			eCh, err = eClient.Stream(ctx, topics, idx, &nomad.QueryOptions{})
			if err != nil {
				log.Fatalf("error subscribing to event stream: %v", err)
			}
		case es := <-eCh:
			if eErrs > 5 {
				log.Fatalf("too many errors in event stream")
			}
			if es.Err != nil && strings.Contains(es.Err.Error(), "invalid character 's' looking for beginning of value") {
				log.Warn("server disconnected, resubscribing")
				cancel()
				clearECh(eCh)
				sub <- true
				continue
			}
			if es.Err != nil {
				log.Error("error in event stream: %v", es.Err)
				eErrs++
				continue
			}

			for _, e := range es.Events {
				log.Debugf("==> idx: %v, topic: %v, type: %v", e.Index, e.Topic, e.Type)

				idx = e.Index

				if e.Type != "AllocationUpdated" {
					continue
				}

				alloc, err := e.Allocation()
				if err != nil {
					log.Errorf("error getting allocation from event stream: %v", err)
					eErrs++
					continue
				}
				if alloc == nil {
					log.Errorf("allocation in event stream shouldn't be nil")
					eErrs++
					continue
				}

				log.Debugf("  |-> task group: %v, client status: %v", alloc.TaskGroup, alloc.ClientStatus)
				for t, ts := range alloc.TaskStates {
					log.Debugf("  |-> task: %v, state: %v, restarts: %v, failed: %v", t, ts.State, ts.Restarts, ts.Failed)
				}

				// workaround for alloc.Stub() to work
				alloc.Job = pc.Job

				allocStub := alloc.Stub()
				allocStubStore[alloc.ID] = allocStub

				log.Debugf("alloc store size %v", len(allocStubStore))

				allocList := make([]*nomad.AllocationListStub, 0, len(allocStubStore))
				for _, v := range allocStubStore {
					allocList = append(allocList, v)
				}

				if tgDone(allocList, groups, true) {
					log.Info("all dependent task groups finished successfully")
					return
				}
			}
		}
	}
}

func (pc *PipelineController) Next(groups []string, dynTasks string) {
	jAllocs, _, err := pc.JobsAPI.Allocations(pc.JobID, true, nil)
	if err != nil {
		log.Fatalf("error getting job allocations: %v", err)
	}

	cAlloc, _, err := pc.AllocsAPI.Info(pc.AllocID, nil)
	if err != nil {
		log.Fatalf("error getting current allocation: %v", err)
	}

	cGroup := pc.Job.LookupTaskGroup(pc.GroupName)
	if cGroup == nil {
		log.Fatalf("could not find current group (%v), this shouldn't happen!", pc.GroupName)
	}

	cTasks := []string{}

	for _, t := range cGroup.Tasks {
		if t.Name != "init" && t.Name != "next" {
			cTasks = append(cTasks, t.Name)
		}
	}

	for _, t := range cTasks {
		if !successState(cAlloc.TaskStates[t]) {
			log.Fatalf("task %v didn't run successfully, not triggering next group", t)
		}
	}

	if len(dynTasks) > 0 {
		glob := filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), dynTasks)
		tgsFiles, err := filepath.Glob(glob)
		if err != nil {
			log.Fatalf("error finding dynamic tasks files using provided glob (%v): %v", dynTasks, err)
		}

		log.Infof("found following dynamic tasks files: %v", tgsFiles)

		tgs := make(TaskGroups, 0)
		for _, tgsFile := range tgsFiles {
			tgsBytes, err := os.ReadFile(tgsFile)
			if err != nil {
				log.Fatalf("error reading dynamic tasks file at path (%v): %v", tgsFile, err)
			}

			var _tgs TaskGroups
			err = json.Unmarshal(tgsBytes, &_tgs)
			if err != nil {
				log.Fatalf("error parsing dynamic tasks json at path (%v): %v", tgsFile, err)
			}

			tgs = append(tgs, _tgs...)
		}

		log.Debugf("dynamic tasks to add: %v", tgs)

		for _, _tg := range tgs {
			tg := _tg

			tg.SetMeta(_parentTask, pc.GroupName)
			pc.Job.AddTaskGroup(&tg)
		}

		filter := map[string]string{
			_parentTask: pc.GroupName,
		}

		rTasks, err := pc.ProcessTaskGroups(filter)
		if err != nil {
			log.Fatalf("error processing task groups: %v", err)
		}

		if len(rTasks) == 0 {
			log.Fatalf("no root task group found, atleast one task in dynamic tasks must have root meta tag (%v)", rootTag)
		}

		groups = append(groups, rTasks...)
	}

	for _, group := range groups {
		tg := pc.Job.LookupTaskGroup(group)
		if tg == nil {
			log.Warnf("could not find next group %v", group)
			continue
		}
		if tgAllocated(jAllocs, []string{group}) && !tgDone(jAllocs, []string{group}, false) {
			log.Warnf("next group already has allocations, skipping trigger: %v", group)
			continue
		}

		count := 1
		if countStr, ok := tg.Meta[countTag]; ok {
			counte := os.ExpandEnv(countStr)
			count, err = strconv.Atoi(counte)
			if err != nil {
				log.Warn("can't convert count tag (%v) of value (%v) to a integer, defaulting to 1", countTag, countStr)
				count = 1
			}
		}
		tg.Count = i2p(count)
	}

	if pc.TaskName == "init" || tgDone(jAllocs, []string{pc.GroupName}, true) {
		cGroup.Count = i2p(0)
	}

	err = pc.UpdateJob()
	if err != nil {
		log.Fatalf("error updating job: %v", err)
	}
}

func main() {
	cPath := flag.String("config", "config.yaml", "path to config")
	init := flag.Bool("init", false, "initialize job with nomad-pipeline hooks")
	wait := flag.Bool("wait", false, "wait for previous task group(s)")
	next := flag.Bool("next", false, "trigger next task group(s)")

	// next args
	dynamicTasks := flag.String("dynamic-tasks", "", "glob of task files relative to alloc dir")

	flag.Parse()

	pc := NewPipelineController(*cPath)

	if *init {
		pc.Init()
		log.Info("successfully initialized job")
		os.Exit(0)
	}

	groups := flag.Args()
	log.Infof("targeting following groups: %v", groups)

	if *wait {
		pc.Wait(groups)
	} else if *next {
		pc.Next(groups, *dynamicTasks)
	}
}