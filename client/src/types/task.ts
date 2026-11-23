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

export interface Subtask {
  id: number;
  title: string;
  done: boolean;
  position: number;
}

// GET /tasks/:id
export interface TaskDetail extends ProjectTask {
  project_name: string;
  creator: TaskUserBrief | null; // null if the creator's account was deleted
  subtasks: Subtask[];
  can_edit: boolean;
  can_delete: boolean; // creator, or admin and above
}

// Body for PUT /tasks/:id. Omit `assignees` to leave them unchanged.
export interface UpdateTaskInput {
  title: string;
  description: string;
  priority: TaskPriority;
  due_date: string | null; // null clears it
  assignees?: number[];
}