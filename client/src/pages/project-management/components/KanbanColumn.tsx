import { Plus } from "lucide-react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import type { ProjectTask, TaskStatus } from "../../../types/task";
import SortableTaskCard from "../SortableTaskCard";

interface KanbanColumnProps {
  status: TaskStatus;
  label: string;
  headerClass: string;
  tasks: ProjectTask[]; // already sorted by position
  highlight: boolean; // a card is being dragged over this column
  dragDisabled: boolean;
  onAdd: () => void;
}

export default function KanbanColumn({
  status,
  label,
  headerClass,
  tasks,
  highlight,
  dragDisabled,
  onAdd,
}: KanbanColumnProps) {
  // Makes the whole column a drop target, so empty columns and the space below the last card work.
  const { setNodeRef } = useDroppable({ id: status });

  return (
    <section className="flex flex-col gap-3">
      <header className={`flex items-center justify-between px-4 py-3 rounded-xl ${headerClass}`}>
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold">{label}</h3>
          <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-white/70">
            {tasks.length}
          </span>
        </div>
        <button
          type="button"
          onClick={onAdd}
          aria-label={`Add task to ${label}`}
          className="p-1 rounded-md hover:bg-white/60"
        >
          <Plus className="w-4 h-4" />
        </button>
      </header>

      <div
        ref={setNodeRef}
        className={`flex flex-col gap-3 min-h-32 rounded-xl transition-colors ${
          highlight ? "bg-emerald-50/60 ring-2 ring-emerald-200" : ""
        }`}
      >
        <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
          {tasks.length === 0 ? (
            <p className="py-6 text-xs text-center text-gray-400 border border-gray-200 border-dashed rounded-xl">
              No tasks
            </p>
          ) : (
            tasks.map((t) => <SortableTaskCard key={t.id} task={t} disabled={dragDisabled} />)
          )}
        </SortableContext>
      </div>
    </section>
  );
}