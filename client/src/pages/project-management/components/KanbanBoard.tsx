import { useMemo, useState } from "react";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import { sortableKeyboardCoordinates } from "@dnd-kit/sortable";
import type { ProjectTask, TaskStatus } from "../../../types/task";
import { columns } from "../taskColumns";
import KanbanColumn from "./KanbanColumn";
import TaskCard from "./TaskCard";

interface KanbanBoardProps {
  tasks: ProjectTask[];
  dragDisabled: boolean;
  onAddTask: (status: TaskStatus) => void;
  onMoveTask: (taskId: number, status: TaskStatus, position: number) => void;
  onOpenTask: (taskId: number) => void;
}

export default function KanbanBoard({ tasks, dragDisabled, onAddTask, onMoveTask, onOpenTask }: KanbanBoardProps) {
  const [activeId, setActiveId] = useState<number | null>(null);
  const [overStatus, setOverStatus] = useState<TaskStatus | null>(null);

  const sensors = useSensors(
    // A 6px threshold keeps plain clicks on a card from starting a drag.
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    // A short press-and-hold on touch screens, so scrolling still works.
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  // Tasks grouped by column and sorted by position.
  const byColumn = useMemo(() => {
    const map = new Map<TaskStatus, ProjectTask[]>(columns.map((c) => [c.status, []]));
    for (const t of tasks) map.get(t.status)?.push(t);
    for (const list of map.values()) list.sort((a, b) => a.position - b.position || a.id - b.id);
    return map;
  }, [tasks]);

  const activeTask = activeId === null ? null : tasks.find((t) => t.id === activeId) ?? null;

  // Task ids are numbers and column ids are status strings, so they can't collide.
  function statusOf(id: UniqueIdentifier): TaskStatus | null {
    if (typeof id === "number") return tasks.find((t) => t.id === id)?.status ?? null;
    return columns.some((c) => c.status === id) ? (id as TaskStatus) : null;
  }

  function reset() {
    setActiveId(null);
    setOverStatus(null);
  }

  function handleDragStart(e: DragStartEvent) {
    setActiveId(Number(e.active.id));
  }

  function handleDragOver(e: DragOverEvent) {
    setOverStatus(e.over ? statusOf(e.over.id) : null);
  }

  function handleDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    reset();
    if (!over) return;

    const taskId = Number(active.id);
    const task = tasks.find((t) => t.id === taskId);
    const destStatus = statusOf(over.id);
    if (!task || !destStatus) return;

    const destList = byColumn.get(destStatus) ?? [];
    const sameColumn = destStatus === task.status;
    const fromIndex = sameColumn ? destList.findIndex((t) => t.id === taskId) : -1;

    let destIndex: number;
    if (typeof over.id === "number") {
      // Dropped on a card.
      const overIndex = destList.findIndex((t) => t.id === over.id);
      if (sameColumn) {
        destIndex = overIndex; // the card ends up where the target card was
      } else {
        // Another column: before the target card, or after it if dropped on its lower half.
        const translated = active.rect.current.translated;
        const below = translated ? translated.top > over.rect.top + over.rect.height / 2 : false;
        destIndex = overIndex + (below ? 1 : 0);
      }
    } else {
      // Dropped on the column itself (empty space or an empty column): go to the end.
      destIndex = sameColumn ? destList.length - 1 : destList.length;
    }

    if (sameColumn && destIndex === fromIndex) return; // nothing moved
    onMoveTask(taskId, destStatus, destIndex);
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
      onDragCancel={reset}
    >
      <div className="grid items-start grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        {columns.map((col) => (
          <KanbanColumn
            key={col.status}
            status={col.status}
            label={col.label}
            headerClass={col.headerClass}
            tasks={byColumn.get(col.status) ?? []}
            highlight={activeId !== null && overStatus === col.status}
            dragDisabled={dragDisabled}
            onAdd={() => onAddTask(col.status)}
            onOpenTask={onOpenTask}
          />
        ))}
      </div>

      {/* The card that follows the pointer, so it can cross columns cleanly. */}
      <DragOverlay>
        {activeTask ? (
          <div className="rotate-2 shadow-xl">
            <TaskCard task={activeTask} />
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}