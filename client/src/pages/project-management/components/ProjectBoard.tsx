import { useCallback, useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";
import { useProjectTasks } from "../../../hooks/useProjectTasks";
import KanbanBoard from "./KanbanBoard";
import type { TaskStatus } from "../../../types/task";
import CreateTaskDrawer from "../CreateTaskDrawer";
import { useMatch, useNavigate } from "react-router-dom";
import TaskDetailDrawer from "../TaskDetailDrawer";

interface ProjectBoardProps {
  projectId: number;
}

export default function ProjectBoard({ projectId }: ProjectBoardProps) {
  const { tasks, truncated, loading, error, refetch, addTask, moveTask, updateTask, removeTask } =
    useProjectTasks(projectId);

  const navigate = useNavigate();
  const taskMatch = useMatch("/projects/:projectId/tasks/:taskId");
  const openTaskId = taskMatch ? Number(taskMatch.params.taskId) : null;
  const openTask = useCallback(
    (id: number) => navigate(`/projects/${projectId}/tasks/${id}`),
    [navigate, projectId]
  );
  const closeTask = useCallback(() => navigate(`/projects/${projectId}`), [navigate, projectId]);

  const [drawerStatus, setDrawerStatus] = useState<TaskStatus | null>(null); // null = closed
  const [notice, setNotice] = useState<string | null>(null);

  const [query, setQuery] = useState("");
  const searching = query.trim() !== "";

  // Filtering happens in the browser: the endpoint returns the whole project.
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return tasks;
    return tasks.filter(
      (t) => t.title.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)
    );
  }, [tasks, query]);

  const handleMove = useCallback(
    async (taskId: number, status: TaskStatus, position: number) => {
      try {
        await moveTask(taskId, status, position);
        setNotice(null);
      } catch {
        setNotice("Couldn't move that task. The board has been refreshed.");
      }
    },
    [moveTask]
  );

  if (loading) {
    return (
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-64 bg-white border border-gray-200 rounded-xl animate-pulse" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-16 bg-white border border-gray-200 rounded-xl">
        <p className="mb-4 text-sm text-gray-600">{error}</p>
        <button
          onClick={refetch}
          className="px-4 py-2 text-sm font-medium text-white rounded-lg bg-emerald-600 hover:bg-emerald-700"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div className="relative w-full max-w-sm">
          <Search className="absolute w-4 h-4 text-gray-400 -translate-y-1/2 left-3 top-1/2" />
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search tasks…"
            className="w-full py-2 pl-9 pr-3 text-sm bg-white border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500"
          />
        </div>
        <span className="text-sm text-gray-500">
          {visible.length} {visible.length === 1 ? "task" : "tasks"}
        </span>
        <button
          type="button"
          onClick={() => setDrawerStatus("pending")}
          className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white rounded-lg bg-emerald-600 hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2"
        >
          <Plus className="w-4 h-4" />
          Create Task
        </button>
      </div>

      {truncated && (
        <p className="px-4 py-2 text-sm rounded-lg bg-amber-50 text-amber-700">
          This project has more than 500 tasks. Showing the first 500. Reordering is turned off.
        </p>
      )}

      {searching && (
        <p className="text-xs text-gray-400">Clear the search to rearrange tasks.</p>
      )}

      {notice && (
        <p className="flex items-center justify-between px-4 py-2 text-sm text-red-700 rounded-lg bg-red-50">
          {notice}
          <button type="button" onClick={() => setNotice(null)} className="font-medium">
            Dismiss
          </button>
        </p>
      )}

      <KanbanBoard
        tasks={visible}
        dragDisabled={searching || truncated}
        onAddTask={setDrawerStatus}
        onMoveTask={handleMove}
        onOpenTask={openTask}
      />

      {drawerStatus && (
        <CreateTaskDrawer
          projectId={projectId}
          defaultStatus={drawerStatus}
          onClose={() => setDrawerStatus(null)}
          onCreated={(task) => {
            addTask(task);
            setDrawerStatus(null);
          }}
        />
      )}

      {openTaskId !== null && Number.isFinite(openTaskId) && (
        <TaskDetailDrawer
          key={openTaskId}
          projectId={projectId}
          taskId={openTaskId}
          onClose={closeTask}
          onUpdated={updateTask}
          onDeleted={(id) => {
            removeTask(id);
            closeTask();
          }}
        />
      )}
    </div>
  );
}