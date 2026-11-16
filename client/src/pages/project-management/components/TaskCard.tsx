import { Clock, Flag, ListChecks, MessageSquare } from "lucide-react";
import type { ProjectTask, TaskPriority } from "../../../types/task";
import AvatarStack from "./AvatarStack";

const priorityStyles: Record<TaskPriority, string> = {
  high: "bg-red-50 text-red-600",
  medium: "bg-amber-50 text-amber-600",
  low: "bg-sky-50 text-sky-600",
};

const formatDue = (iso: string) =>
  new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });

interface TaskCardProps {
  task: ProjectTask;
}

export default function TaskCard({ task }: TaskCardProps) {
  const { title, description, status, priority, due_date, assignees, subtask_total, subtask_done, comment_count } = task;
  const overdue = due_date !== null && status !== "done" && new Date(due_date) < new Date();
  const pct = subtask_total > 0 ? Math.round((subtask_done / subtask_total) * 100) : 0;

  return (
    <article className="flex flex-col gap-3 p-4 bg-white border border-gray-200 rounded-xl">
      {due_date && (
        <span
          className={`inline-flex items-center self-start gap-1.5 text-xs ${
            overdue ? "text-red-600" : "text-gray-500"
          }`}
        >
          <Clock className="w-3.5 h-3.5" />
          Due: {formatDue(due_date)}
        </span>
      )}

      <h4 className="font-medium text-gray-900 line-clamp-2">{title}</h4>
      {description && <p className="text-sm text-gray-500 line-clamp-2">{description}</p>}

      {subtask_total > 0 && (
        <div>
          <div className="flex items-center justify-between mb-1.5 text-xs text-gray-500">
            <span className="inline-flex items-center gap-1.5">
              <ListChecks className="w-3.5 h-3.5" />
              Subtasks
            </span>
            <span>
              {subtask_done}/{subtask_total}
            </span>
          </div>
          <div className="h-1.5 overflow-hidden bg-gray-100 rounded-full">
            <div className="h-full rounded-full bg-emerald-500" style={{ width: `${pct}%` }} />
          </div>
        </div>
      )}

      {assignees.length > 0 ? (
        <AvatarStack members={assignees.slice(0, 3)} total={assignees.length} />
      ) : (
        <span className="text-xs text-gray-400">Unassigned</span>
      )}

      <div className="flex items-center justify-between pt-3 border-t border-gray-100">
        <span
          className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium capitalize rounded-full ${priorityStyles[priority]}`}
        >
          <Flag className="w-3 h-3" />
          {priority}
        </span>
        <span className="inline-flex items-center gap-1 text-xs text-gray-500">
          <MessageSquare className="w-3.5 h-3.5" />
          {comment_count}
        </span>
      </div>
    </article>
  );
}