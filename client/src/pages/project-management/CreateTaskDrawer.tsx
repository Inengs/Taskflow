import { FormEvent, useEffect, useState } from "react";
import axios from "axios";
import { X } from "lucide-react";
import { createProjectTask } from "../../api/projects";
import { useProjectMembers } from "../../hooks/useProjectMembers";
import type { ProjectTask, TaskPriority, TaskStatus } from "../../types/task";
import { columns } from "./taskColumns";

const priorities: { value: TaskPriority; label: string; active: string }[] = [
  { value: "low", label: "Low", active: "bg-sky-50 text-sky-700 border-sky-300" },
  { value: "medium", label: "Medium", active: "bg-amber-50 text-amber-700 border-amber-300" },
  { value: "high", label: "High", active: "bg-red-50 text-red-700 border-red-300" },
];

interface CreateTaskDrawerProps {
  projectId: number;
  defaultStatus: TaskStatus;
  onClose: () => void;
  onCreated: (task: ProjectTask) => void;
}

export default function CreateTaskDrawer({
  projectId,
  defaultStatus,
  onClose,
  onCreated,
}: CreateTaskDrawerProps) {
  const { members, loading: membersLoading, error: membersError, refetch: refetchMembers } =
    useProjectMembers(projectId);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [status, setStatus] = useState<TaskStatus>(defaultStatus);
  const [priority, setPriority] = useState<TaskPriority>("medium");
  const [dueDate, setDueDate] = useState(""); // "YYYY-MM-DD" from <input type="date">
  const [assignees, setAssignees] = useState<number[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Close on Escape
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const toggleAssignee = (id: number) =>
    setAssignees((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!title.trim()) {
      setError("Task title is required");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const { task } = await createProjectTask(projectId, {
        title: title.trim(),
        description: description.trim(),
        status,
        priority,
        // Noon UTC keeps the calendar date the same in every timezone (midnight would
        // show as the previous day for anyone west of UTC).
        due_date: dueDate ? `${dueDate}T12:00:00Z` : null,
        assignees,
      });
      onCreated(task);
    } catch (err) {
      const data = axios.isAxiosError(err) ? err.response?.data : null;
      setError(data?.error ?? data?.errors?.join(", ") ?? "Could not create the task");
      setSubmitting(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onClick={onClose}>
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-task-title"
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="flex flex-col w-full h-full max-w-md bg-white shadow-xl"
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200">
          <h2 id="create-task-title" className="text-lg font-semibold text-gray-900">
            Create task
          </h2>
          <button type="button" onClick={onClose} aria-label="Close" className="text-gray-400 hover:text-gray-600">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex-1 px-6 py-5 space-y-5 overflow-y-auto">
          <div>
            <label htmlFor="task-title" className="block mb-1 text-sm font-medium text-gray-700">
              Title
            </label>
            <input
              id="task-title"
              autoFocus
              maxLength={255}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Design the onboarding screens"
              className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500"
            />
          </div>

          <div>
            <label htmlFor="task-description" className="block mb-1 text-sm font-medium text-gray-700">
              Description <span className="font-normal text-gray-400">(optional)</span>
            </label>
            <textarea
              id="task-description"
              rows={4}
              maxLength={10000}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-emerald-500"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label htmlFor="task-status" className="block mb-1 text-sm font-medium text-gray-700">
                Status
              </label>
              <select
                id="task-status"
                value={status}
                onChange={(e) => setStatus(e.target.value as TaskStatus)}
                className="w-full px-3 py-2 text-sm bg-white border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500"
              >
                {columns.map((c) => (
                  <option key={c.status} value={c.status}>
                    {c.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="task-due" className="block mb-1 text-sm font-medium text-gray-700">
                Due date
              </label>
              <input
                id="task-due"
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
                className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500"
              />
            </div>
          </div>

          <div>
            <span className="block mb-1 text-sm font-medium text-gray-700">Priority</span>
            <div className="flex gap-2">
              {priorities.map((p) => (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => setPriority(p.value)}
                  aria-pressed={priority === p.value}
                  className={`flex-1 px-3 py-2 text-sm font-medium border rounded-lg ${
                    priority === p.value
                      ? p.active
                      : "text-gray-600 border-gray-200 hover:bg-gray-50"
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <span className="block mb-1 text-sm font-medium text-gray-700">
              Assignees <span className="font-normal text-gray-400">(optional)</span>
            </span>
            <div className="overflow-y-auto border border-gray-200 divide-y divide-gray-100 rounded-lg max-h-56">
              {membersLoading ? (
                <p className="px-3 py-3 text-sm text-gray-400">Loading members…</p>
              ) : membersError ? (
                <div className="flex items-center justify-between px-3 py-3 text-sm text-gray-600">
                  <span>{membersError}</span>
                  <button type="button" onClick={refetchMembers} className="font-medium text-emerald-600">
                    Retry
                  </button>
                </div>
              ) : (
                members.map((m) => (
                  <label key={m.id} className="flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-gray-50">
                    <input
                      type="checkbox"
                      checked={assignees.includes(m.id)}
                      onChange={() => toggleAssignee(m.id)}
                      className="accent-emerald-600"
                    />
                    {m.avatar_url ? (
                      <img src={m.avatar_url} alt="" className="object-cover w-7 h-7 rounded-full" />
                    ) : (
                      <span className="flex items-center justify-center text-xs font-medium text-gray-600 uppercase bg-gray-100 rounded-full w-7 h-7">
                        {m.username.charAt(0)}
                      </span>
                    )}
                    <span className="text-sm text-gray-800">{m.username}</span>
                    <span className="ml-auto text-xs text-gray-400 capitalize">{m.role}</span>
                  </label>
                ))
              )}
            </div>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}
        </div>

        <div className="flex justify-end gap-2 px-6 py-4 border-t border-gray-200">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-gray-700 rounded-lg hover:bg-gray-100"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-2 text-sm font-medium text-white rounded-lg bg-emerald-600 hover:bg-emerald-700 disabled:opacity-60"
          >
            {submitting ? "Creating…" : "Create task"}
          </button>
        </div>
      </form>
    </div>
  );
}