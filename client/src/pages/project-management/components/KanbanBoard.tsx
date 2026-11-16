import type { ProjectTask, TaskStatus } from "../../../types/task";
import KanbanColumn from "./KanbanColumn";

const columns: { status: TaskStatus; label: string; headerClass: string }[] = [
  { status: "pending", label: "To Do", headerClass: "bg-sky-50 text-sky-700" },
  { status: "in-progress", label: "In Progress", headerClass: "bg-amber-50 text-amber-700" },
  { status: "in-review", label: "In Review", headerClass: "bg-rose-50 text-rose-700" },
  { status: "done", label: "Completed", headerClass: "bg-emerald-50 text-emerald-700" },
];

interface KanbanBoardProps {
  tasks: ProjectTask[];
}

export default function KanbanBoard({ tasks }: KanbanBoardProps) {
  return (
    <div className="grid items-start grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      {columns.map((col) => (
        <KanbanColumn
          key={col.status}
          label={col.label}
          headerClass={col.headerClass}
          tasks={tasks.filter((t) => t.status === col.status)}
        />
      ))}
    </div>
  );
}