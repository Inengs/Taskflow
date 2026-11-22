import type { ProjectTask, TaskStatus } from "../types/task";

// Returns the task list as it will look after moving a task to `position` in `status`.
export function applyTaskMove(
  tasks: ProjectTask[],
  taskId: number,
  status: TaskStatus,
  position: number
): ProjectTask[] {
  const moving = tasks.find((t) => t.id === taskId);
  if (!moving) return tasks;
  const oldStatus = moving.status;
  const oldPos = moving.position;

  return tasks.map((t) => {
    if (t.id === taskId) return { ...t, status, position };

    if (oldStatus === status) {
      // Same-column reorder
      if (t.status !== status) return t;
      if (position < oldPos && t.position >= position && t.position < oldPos) {
        return { ...t, position: t.position + 1 }; // moving up: make room
      }
      if (position > oldPos && t.position > oldPos && t.position <= position) {
        return { ...t, position: t.position - 1 }; // moving down: close the gap
      }
      return t;
    }

    // Cross-column move
    if (t.status === oldStatus && t.position > oldPos) return { ...t, position: t.position - 1 };
    if (t.status === status && t.position >= position) return { ...t, position: t.position + 1 };
    return t;
  });
}