import type { ProjectMember } from "../../../types/project";

interface AssigneePickerProps {
  members: ProjectMember[];
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  selected: number[];
  onToggle: (id: number) => void;
  disabled?: boolean;
}

export default function AssigneePicker({
  members,
  loading,
  error,
  onRetry,
  selected,
  onToggle,
  disabled = false,
}: AssigneePickerProps) {
  return (
    <div className="overflow-y-auto border border-gray-200 divide-y divide-gray-100 rounded-lg max-h-56">
      {loading ? (
        <p className="px-3 py-3 text-sm text-gray-400">Loading members…</p>
      ) : error ? (
        <div className="flex items-center justify-between px-3 py-3 text-sm text-gray-600">
          <span>{error}</span>
          <button type="button" onClick={onRetry} className="font-medium text-emerald-600">
            Retry
          </button>
        </div>
      ) : (
        members.map((m) => (
          <label key={m.id} className="flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-gray-50">
            <input
              type="checkbox"
              checked={selected.includes(m.id)}
              onChange={() => onToggle(m.id)}
              disabled={disabled}
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
  );
}