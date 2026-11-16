import type { ProjectTask } from "../../../types/task";
import TaskCard from "./TaskCard";

interface KanbanColumnProps {
  label: string;
  headerClass: string;
  tasks: ProjectTask[];
}

export default function KanbanColumn({ label, headerClass, tasks }: KanbanColumnProps) {
  return (
    <section className="flex flex-col gap-3">
      <header className={`flex items-center justify-between px-4 py-3 rounded-xl ${headerClass}`}>
        <h3 className="text-sm font-semibold">{label}</h3>
        <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-white/70">
          {tasks.length}
        </span>
      </header>

      {tasks.length === 0 ? (
        <p className="py-6 text-xs text-center text-gray-400 border border-gray-200 border-dashed rounded-xl">
          No tasks
        </p>
      ) : (
        tasks.map((t) => <TaskCard key={t.id} task={t} />)
      )}
    </section>
  );
}