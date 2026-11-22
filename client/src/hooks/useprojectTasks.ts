import { useCallback, useEffect, useRef, useState } from "react";
import { getProjectTasks } from "../api/projects";
import { updateTaskStatus } from "../api/tasks";
import { applyTaskMove } from "../utils/applyTaskMove";
import type { ProjectTask, TaskStatus } from "../types/task";

interface State {
  tasks: ProjectTask[];
  truncated: boolean; // the server capped the list at 500
  loading: boolean;
  error: string | null;
}

export function useProjectTasks(projectId: number) {
  const [state, setState] = useState<State>({
    tasks: [],
    truncated: false,
    loading: true,
    error: null,
  });
  // Ignore responses from an older request (e.g. after switching projects quickly).
  const latest = useRef(0);
  // Moves are sent one at a time, in order, so the server applies them as the user made them.
  const moveQueue = useRef<Promise<unknown>>(Promise.resolve());

  const refetch = useCallback(async () => {
    const requestId = ++latest.current;
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const { tasks, truncated } = await getProjectTasks(projectId);
      if (requestId !== latest.current) return;
      setState({ tasks, truncated, loading: false, error: null });
    } catch (err) {
      if (requestId !== latest.current) return;
      setState({
        tasks: [],
        truncated: false,
        loading: false,
        error: err instanceof Error ? err.message : "Failed to load tasks",
      });
    }
  }, [projectId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  // Reload from the server without the loading skeleton (used to undo a failed move).
  const resync = useCallback(async () => {
    const requestId = ++latest.current;
    try {
      const { tasks, truncated } = await getProjectTasks(projectId);
      if (requestId !== latest.current) return;
      setState((s) => ({ ...s, tasks, truncated }));
    } catch {
      // Keep what's on screen; the next action will try again.
    }
  }, [projectId]);

  // Mirrors the server: a new task goes to the top of its column,
  // and the other tasks in that column shift down by one.
  const addTask = useCallback((task: ProjectTask) => {
    setState((s) => ({
      ...s,
      tasks: [
        task,
        ...s.tasks.map((t) => (t.status === task.status ? { ...t, position: t.position + 1 } : t)),
      ],
    }));
  }, []);

  // Optimistic move. Throws (after re-syncing) if the server rejects it.
  const moveTask = useCallback(
    async (taskId: number, status: TaskStatus, position: number) => {
      setState((s) => ({ ...s, tasks: applyTaskMove(s.tasks, taskId, status, position) }));

      const request = moveQueue.current.then(() => updateTaskStatus(taskId, { status, position }));
      moveQueue.current = request.catch(() => undefined); // a failure must not block later moves
      try {
        await request;
      } catch (err) {
        await resync();
        throw err;
      }
    },
    [resync]
  );

  return { ...state, refetch, addTask, moveTask };
}