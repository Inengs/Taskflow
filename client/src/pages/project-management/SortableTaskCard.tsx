import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { ProjectTask } from "../../types/task";
import TaskCard from "./components/TaskCard";

interface SortableTaskCardProps {
  task: ProjectTask;
  disabled: boolean;
  onOpen: () => void;
}

export default function SortableTaskCard({ task, disabled, onOpen }: SortableTaskCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: task.id,
    disabled,
  });

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      {...attributes}
      {...listeners}
      onClick={onOpen} // new: a plain click opens the task
      className={`${isDragging ? "opacity-40" : ""} ${
        disabled ? "" : "cursor-grab active:cursor-grabbing"
      }`}
    >
      <TaskCard task={task} />
    </div>
  );
}