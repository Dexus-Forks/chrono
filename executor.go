package chrono

import (
	"sync"
	"time"
)

type ScheduledExecutor interface {
	Schedule(task Task, delay time.Duration) *ScheduledTask
	ScheduleWithFixedDelay(task Task, initialDelay time.Duration, delay time.Duration) *ScheduledTask
	ScheduleAtWithRate(task Task, initialDelay time.Duration, period time.Duration) *ScheduledTask
}

type ScheduledTaskExecutor struct {
	nextSequence      int
	nextSequenceMu    sync.RWMutex
	timer             *time.Timer
	taskQueue         ScheduledTaskQueue
	taskQueueMu       sync.RWMutex
	newTaskChannel    chan *ScheduledTask
	removeTaskChannel chan *ScheduledTask
	taskWaitGroup     sync.WaitGroup
}

func NewScheduledTaskExecutor() *ScheduledTaskExecutor {
	executor := &ScheduledTaskExecutor{
		timer:             time.NewTimer(1 * time.Hour),
		taskQueue:         make(ScheduledTaskQueue, 0),
		newTaskChannel:    make(chan *ScheduledTask),
		removeTaskChannel: make(chan *ScheduledTask),
	}

	executor.timer.Stop()

	go executor.run()

	return executor
}

func (executor *ScheduledTaskExecutor) Schedule(task Task, delay time.Duration) *ScheduledTask {
	if task == nil {
		panic("task cannot be nil")
	}

	scheduledTask := NewScheduledTask(task, executor.calculateTriggerTime(delay), 0, false)
	executor.addNewTask(scheduledTask)

	return scheduledTask
}

func (executor *ScheduledTaskExecutor) ScheduleWithFixedDelay(task Task, initialDelay time.Duration, delay time.Duration) *ScheduledTask {
	if task == nil {
		panic("task cannot be nil")
	}

	scheduledTask := NewScheduledTask(task, executor.calculateTriggerTime(initialDelay), delay, false)
	executor.addNewTask(scheduledTask)

	return scheduledTask
}

func (executor *ScheduledTaskExecutor) ScheduleAtWithRate(task Task, initialDelay time.Duration, period time.Duration) *ScheduledTask {
	if task == nil {
		panic("task cannot be nil")
	}

	scheduledTask := NewScheduledTask(task, executor.calculateTriggerTime(initialDelay), period, true)
	executor.addNewTask(scheduledTask)

	return scheduledTask
}

func (executor *ScheduledTaskExecutor) calculateTriggerTime(delay time.Duration) time.Time {
	if delay < 0 {
		delay = 0
	}

	return time.Now().Add(delay)
}

func (executor *ScheduledTaskExecutor) addNewTask(task *ScheduledTask) {
	executor.newTaskChannel <- task
}

func (executor *ScheduledTaskExecutor) run() {

	lastClock := time.Now()

	for {

		if executor.taskQueue.IsEmpty() {
			executor.timer.Stop()
		} else {
			executor.timer.Reset(executor.taskQueue[0].GetDelay())
		}

		for {
			select {
			case clock := <-executor.timer.C:
				executor.timer.Stop()

				executor.taskQueueMu.Lock()

				for index, scheduledTask := range executor.taskQueue {

					if lastClock.After(scheduledTask.triggerTime) {
						continue
					}

					if scheduledTask.triggerTime.After(clock) || scheduledTask.triggerTime.IsZero() {
						break
					}

					if scheduledTask.IsFixedRate() {
						scheduledTask.triggerTime = scheduledTask.triggerTime.Add(scheduledTask.period)
					}

					if !scheduledTask.IsPeriodic() || !scheduledTask.IsFixedRate() {
						executor.taskQueue = append(executor.taskQueue[:index], executor.taskQueue[index+1:]...)
					}

					executor.startTask(scheduledTask)
				}

				executor.taskQueue.SorByTriggerTime()
				lastClock = clock

				executor.taskQueueMu.Unlock()
			case newScheduledTask := <-executor.newTaskChannel:
				executor.timer.Stop()

				executor.taskQueueMu.Lock()
				executor.taskQueue = append(executor.taskQueue, newScheduledTask)
				executor.taskQueue.SorByTriggerTime()
				executor.taskQueueMu.Unlock()
			case task := <-executor.removeTaskChannel:
				executor.timer.Stop()

				executor.taskQueueMu.Lock()

				taskIndex := -1
				for index, scheduledTask := range executor.taskQueue {
					if scheduledTask.id == task.id {
						taskIndex = index
						break
					}
				}

				executor.taskQueue = append(executor.taskQueue[:taskIndex], executor.taskQueue[taskIndex+1:]...)
				executor.taskQueueMu.Unlock()
			}

			break
		}

	}

}

func (executor *ScheduledTaskExecutor) startTask(scheduledTask *ScheduledTask) {
	executor.taskWaitGroup.Add(1)

	go func(scheduledTask *ScheduledTask) {
		defer func() {
			executor.taskWaitGroup.Done()

			scheduledTask.triggerTime = executor.calculateTriggerTime(scheduledTask.period)

			if !scheduledTask.IsFixedRate() {
				executor.newTaskChannel <- scheduledTask
			}
		}()

		scheduledTask.task(nil)
	}(scheduledTask)
}
