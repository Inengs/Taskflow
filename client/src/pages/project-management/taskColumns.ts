import type { TaskStatus } from "../../types/task";

export const columns: { status: TaskStatus; label: string; headerClass: string }[] = [
  { status: "pending", label: "To Do", headerClass: "bg-sky-50 text-sky-700" },
  { status: "in-progress", label: "In Progress", headerClass: "bg-amber-50 text-amber-700" },
  { status: "in-review", label: "In Review", headerClass: "bg-rose-50 text-rose-700" },
  { status: "done", label: "Completed", headerClass: "bg-emerald-50 text-emerald-700" },
];