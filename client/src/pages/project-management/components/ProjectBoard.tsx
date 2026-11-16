import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { useProjectTasks } from "../../../hooks/useprojectTasks";
import KanbanBoard from "./KanbanBoard";

interface ProjectBoardProps {
  projectId: number;
}

export default function ProjectBoard({ projectId }: ProjectBoardProps) {
  const { tasks, truncated, loading, error, refetch } = useProjectTasks(projectId);
  const [query, setQuery] = useState("");

  // Filtering happens in the browser: the endpoint returns the whole project.
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return tasks;
    return tasks.filter(
      (t) => t.title.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)
    );
  }, [tasks, query]);

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
        <span className="text-sm text-gray-500 shrink-0">
          {visible.length} {visible.length === 1 ? "task" : "tasks"}
        </span>
      </div>

      {truncated && (
        <p className="px-4 py-2 text-sm rounded-lg bg-amber-50 text-amber-700">
          This project has more than 500 tasks. Showing the first 500.
        </p>
      )}

      <KanbanBoard tasks={visible} />
    </div>
  );
}