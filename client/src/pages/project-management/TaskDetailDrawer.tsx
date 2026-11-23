import { useEffect, useMemo, useState } from "react";
import axios from "axios";
import { Trash2, X } from "lucide-react";
import { deleteTask, updateTask } from "../../api/tasks";
import { useProjectMembers } from "../../hooks/useProjectMembers";
import { useTaskDetail } from "../../hooks/useTaskDetail";
import type { ProjectTask, TaskDetail, TaskPriority } from "../../types/task";
import { columns } from "./taskColumns";
import AssigneePicker from "./components/AssigneePicker";

const priorities: { value: TaskPriority; label: string; active: string }[] = [
  { value: "low", label: "Low", active: "bg-sky-50 text-sky-700 border-sky-300" },
  { value: "medium", label: "Medium", active: "bg-amber-50 text-amber-700 border-amber-300" },
  { value: "high", label: "High", active: "bg-red-50 text-red-700 border-red-300" },
];

interface Form {
  title: string;
  description: string;
  priority: TaskPriority;
  dueDate: string; // "YYYY-MM-DD" or ""
  assignees: number[];
}

const toForm = (t: TaskDetail): Form => ({
  title: t.title,
  description: t.description,
  priority: t.priority,
  dueDate: t.due_date ? t.due_date.slice(0, 10) : "",
  assignees: t.assignees.map((a) => a.id),
});

const sameIds = (a: number[], b: number[]) => {
  if (a.length !== b.length) return false;
  const sb = [...b].sort((x, y) => x - y);
  return [...a].sort((x, y) => x - y).every((v, i) => v === sb[i]);
};

const formatDateTime = (iso: string) =>
  new Date(iso).toLocaleString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

function errorMessage(err: unknown, fallback: string) {
  const data = axios.isAxiosError(err) ? err.response?.data : null;
  return data?.error ?? data?.errors?.join(", ") ?? fallback;
}

interface TaskDetailDrawerProps {
  projectId: number;
  taskId: number;
  onClose: () => void;
  onUpdated: (task: ProjectTask) => void;
  onDeleted: (taskId: number) => void;
}

export default function TaskDetailDrawer({
  projectId,
  taskId,
  onClose,
  onUpdated,
  onDeleted,
}: TaskDetailDrawerProps) {
  const { task, loading, error, notFound, refetch, setTask } = useTaskDetail(taskId);
  const members = useProjectMembers(projectId);

  const [form, setForm] = useState<Form | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // What the form looks like when it is unchanged (re-derived after every save).
  const initial = useMemo(() => (task ? toForm(task) : null), [task]);

  // Fill the form once the task loads.
  useEffect(() => {
    if (task) setForm(toForm(task));
    // Only when a different task finishes loading, not after each save.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [task?.id]);

  const dirty =
    form !== null &&
    initial !== null &&
    (form.title !== initial.title ||
      form.description !== initial.description ||
      form.priority !== initial.priority ||
      form.dueDate !== initial.dueDate ||
      !sameIds(form.assignees, initial.assignees));

  function requestClose() {
    if (dirty && !window.confirm("Discard your unsaved changes?")) return;
    onClose();
  }

  // Close on Escape
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && requestClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const patch = (changes: Partial<Form>) => setForm((f) => (f ? { ...f, ...changes } : f));
  const toggleAssignee = (id: number) =>
    form && patch({ assignees: form.assignees.includes(id) ? form.assignees.filter((x) => x !== id) : [...form.assignees, id] });

  async function handleSave() {
    if (!task || !form || !initial) return;
    if (!form.title.trim()) {
      setActionError("Task title is required");
      return;
    }
    setSaving(true);
    setActionError(null);
    try {
      const { task: updated } = await updateTask(task.id, {
        title: form.title.trim(),
        description: form.description.trim(),
        priority: form.priority,
        // Untouched date: send the original value back, so its time of day isn't altered.
        due_date:
          form.dueDate === initial.dueDate
            ? task.due_date
            : form.dueDate
              ? `${form.dueDate}T12:00:00Z`
              : null,
        // Only send assignees if they changed (omitted = unchanged on the server).
        ...(sameIds(form.assignees, initial.assignees) ? {} : { assignees: form.assignees }),
      });
      const merged: TaskDetail = { ...task, ...updated }; // keeps creator, subtasks and permission flags
      setTask(merged);
      setForm(toForm(merged));
      onUpdated(updated);
    } catch (err) {
      setActionError(errorMessage(err, "Could not save the task"));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    setDeleting(true);
    setActionError(null);
    try {
      await deleteTask(taskId);
      onDeleted(taskId);
    } catch (err) {
      setActionError(errorMessage(err, "Could not delete the task"));
      setDeleting(false);
      setConfirmDelete(false);
    }
  }

  const wrongProject = task !== null && task.project_id !== projectId;
  const statusLabel = task ? columns.find((c) => c.status === task.status)?.label : "";

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onClick={requestClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="task-detail-title"
        onClick={(e) => e.stopPropagation()}
        className="flex flex-col w-full h-full max-w-xl bg-white shadow-xl"
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200">
          <div className="flex items-center gap-2 text-sm text-gray-500">
            <span>{task?.project_name ?? "Task"}</span>
            {task && statusLabel && (
              <span className="px-2 py-0.5 text-xs font-medium text-gray-600 bg-gray-100 rounded-full">
                {statusLabel}
              </span>
            )}
          </div>
          <button type="button" onClick={requestClose} aria-label="Close" className="text-gray-400 hover:text-gray-600">
            <X className="w-5 h-5" />
          </button>
        </div>

        {loading ? (
          <div className="flex-1 px-6 py-5 space-y-4">
            <div className="h-8 bg-gray-100 rounded animate-pulse" />
            <div className="h-24 bg-gray-100 rounded animate-pulse" />
            <div className="h-10 bg-gray-100 rounded animate-pulse" />
          </div>
        ) : notFound || wrongProject ? (
          <div className="flex flex-col items-center justify-center flex-1 gap-3 px-6 text-center">
            <p className="text-sm font-medium text-gray-900">This task doesn't exist or was deleted.</p>
            <button type="button" onClick={onClose} className="text-sm font-medium text-emerald-600">
              Back to the board
            </button>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center flex-1 gap-3 px-6 text-center">
            <p className="text-sm text-gray-600">{error}</p>
            <button type="button" onClick={refetch} className="text-sm font-medium text-emerald-600">
              Retry
            </button>
          </div>
        ) : task && form ? (
          <>
            <div className="flex-1 px-6 py-5 space-y-5 overflow-y-auto">
              <input
                id="task-detail-title"
                aria-label="Title"
                maxLength={255}
                disabled={!task.can_edit}
                value={form.title}
                onChange={(e) => patch({ title: e.target.value })}
                className="w-full px-2 py-1 -mx-2 text-xl font-semibold text-gray-900 border border-transparent rounded-lg hover:border-gray-200 focus:border-gray-200 focus:outline-none focus:ring-2 focus:ring-emerald-500 disabled:bg-transparent"
              />

              <div>
                <label htmlFor="detail-description" className="block mb-1 text-sm font-medium text-gray-700">
                  Description
                </label>
                <textarea
                  id="detail-description"
                  rows={5}
                  maxLength={10000}
                  disabled={!task.can_edit}
                  value={form.description}
                  onChange={(e) => patch({ description: e.target.value })}
                  placeholder="Add a description…"
                  className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <span className="block mb-1 text-sm font-medium text-gray-700">Priority</span>
                  <div className="flex gap-2">
                    {priorities.map((p) => (
                      <button
                        key={p.value}
                        type="button"
                        disabled={!task.can_edit}
                        onClick={() => patch({ priority: p.value })}
                        aria-pressed={form.priority === p.value}
                        className={`flex-1 px-2 py-2 text-sm font-medium border rounded-lg ${
                          form.priority === p.value
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
                  <label htmlFor="detail-due" className="block mb-1 text-sm font-medium text-gray-700">
                    Due date
                  </label>
                  <input
                    id="detail-due"
                    type="date"
                    disabled={!task.can_edit}
                    value={form.dueDate}
                    onChange={(e) => patch({ dueDate: e.target.value })}
                    className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500"
                  />
                </div>
              </div>

              <div>
                <span className="block mb-1 text-sm font-medium text-gray-700">Assignees</span>
                <AssigneePicker
                  members={members.members}
                  loading={members.loading}
                  error={members.error}
                  onRetry={members.refetch}
                  selected={form.assignees}
                  onToggle={toggleAssignee}
                  disabled={!task.can_edit}
                />
              </div>

              <p className="text-xs text-gray-400">
                Created {formatDateTime(task.created_at)}
                {task.creator ? ` by ${task.creator.username}` : ""}
              </p>

              {/* Subtasks and comments go here (next steps). */}

              {actionError && <p className="text-sm text-red-600">{actionError}</p>}
            </div>

            <div className="flex items-center justify-between px-6 py-4 border-t border-gray-200">
              <div>
                {task.can_delete &&
                  (confirmDelete ? (
                    <span className="flex items-center gap-2 text-sm">
                      <span className="text-gray-600">Delete this task?</span>
                      <button
                        type="button"
                        onClick={handleDelete}
                        disabled={deleting}
                        className="font-medium text-red-600 hover:text-red-700 disabled:opacity-60"
                      >
                        {deleting ? "Deleting…" : "Yes, delete"}
                      </button>
                      <button
                        type="button"
                        onClick={() => setConfirmDelete(false)}
                        className="text-gray-500 hover:text-gray-700"
                      >
                        Cancel
                      </button>
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => setConfirmDelete(true)}
                      className="flex items-center gap-1.5 text-sm text-gray-500 hover:text-red-600"
                    >
                      <Trash2 className="w-4 h-4" />
                      Delete
                    </button>
                  ))}
              </div>

              <div className="flex gap-2">
                {dirty && (
                  <button
                    type="button"
                    onClick={() => {
                      setForm(initial);
                      setActionError(null);
                    }}
                    className="px-4 py-2 text-sm font-medium text-gray-700 rounded-lg hover:bg-gray-100"
                  >
                    Discard
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={!dirty || saving || !task.can_edit}
                  className="px-4 py-2 text-sm font-medium text-white rounded-lg bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50"
                >
                  {saving ? "Saving…" : "Save"}
                </button>
              </div>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}