package engine

import (
	"fmt"

	agentkittools "github.com/kayushkin/agentkit/tools"
	"github.com/kayushkin/inber/agent"
)

// buildSidebandCallbacks creates callbacks for task plan and scratchpad operations.
func (e *Engine) buildSidebandCallbacks() *agent.SidebandCallbacks {
	repoRoot := e.repoRoot
	agentName := e.AgentName

	return &agent.SidebandCallbacks{
		CompleteTasks: func(indices []int) error {
			plan := loadTaskPlan(repoRoot)
			if plan == nil {
				return fmt.Errorf("no task plan found")
			}

			// Mark tasks done (iterate in reverse to preserve indices).
			for i := len(indices) - 1; i >= 0; i-- {
				idx := indices[i]
				if idx >= 0 && idx < len(plan.Tasks) {
					plan.Tasks[idx].Status = agentkittools.TaskDone
				}
			}

			// Remove done tasks.
			filtered := plan.Tasks[:0]
			for _, t := range plan.Tasks {
				if t.Status != agentkittools.TaskDone {
					filtered = append(filtered, t)
				}
			}
			plan.Tasks = filtered

			if err := saveTaskPlan(repoRoot, plan); err != nil {
				return err
			}

			// If all tasks done, auto-build.
			if len(plan.Tasks) == 0 {
				result := agentkittools.RunBuildCheck(repoRoot)
				if !result.Success {
					_ = agentkittools.AddBuildErrorTask(repoRoot, result.Output)
				}
			}
			return nil
		},

		SaveNote: func(key, value string) error {
			notes := agentkittools.LoadScratchpadNotes(repoRoot, agentName)
			if notes == nil {
				notes = make(map[string]string)
			}
			notes[key] = value
			return agentkittools.SaveScratchpadNotes(repoRoot, agentName, notes)
		},

		SplitTask: func(index int, subtasks []string) error {
			plan := loadTaskPlan(repoRoot)
			if plan == nil {
				return fmt.Errorf("no task plan found")
			}
			if index < 0 || index >= len(plan.Tasks) {
				return fmt.Errorf("index %d out of range", index)
			}

			// Replace the task at index with subtasks.
			var newTasks []agentkittools.TaskItem
			for i, t := range plan.Tasks {
				if i == index {
					for _, st := range subtasks {
						newTasks = append(newTasks, agentkittools.TaskItem{
							Task:   st,
							Status: agentkittools.TaskPending,
						})
					}
				} else {
					newTasks = append(newTasks, t)
				}
			}
			plan.Tasks = newTasks
			return saveTaskPlan(repoRoot, plan)
		},
	}
}

// loadTaskPlan reads the task plan from disk.
func loadTaskPlan(repoRoot string) *agentkittools.TaskPlan {
	content := agentkittools.LoadPlanContext(repoRoot)
	if content == "" {
		return nil
	}
	return agentkittools.ParsePlanMD(content)
}

// saveTaskPlan writes the task plan to disk.
func saveTaskPlan(repoRoot string, plan *agentkittools.TaskPlan) error {
	return agentkittools.SavePlanMD(repoRoot, plan)
}
