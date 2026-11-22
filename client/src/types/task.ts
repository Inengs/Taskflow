import type { TaskUserBrief } from "./dashboard";

export type TaskStatus = "pending" | "in-progress" | "in-review" | "done";
export type TaskPriority = "low" | "medium" | "high";

// One card on the board (GET /projects/:id/tasks).
export interface ProjectTask {
  id: number;
  project_id: number;
  title: string;
  description: string;
  status: TaskStatus; // "pending" is shown as "To Do"
  priority: TaskPriority;
  due_date: string | null;
  position: number;
  assignees: TaskUserBrief[];
  subtask_total: number;
  subtask_done: number;
  comment_count: number;
  created_at: string;
  updated_at: string;
}

// Body for POST /projects/:id/tasks.
export interface CreateTaskInput {
  title: string;
  description: string;
  status: TaskStatus;
  priority: TaskPriority;
  due_date: string | null; // ISO string, or null for no due date
  assignees: number[]; // user ids; must be project members
}