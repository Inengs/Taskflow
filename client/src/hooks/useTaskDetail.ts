import { useCallback, useEffect, useRef, useState } from "react";
import axios from "axios";
import { getTask } from "../api/tasks";
import type { TaskDetail } from "../types/task";

interface State {
  task: TaskDetail | null;
  loading: boolean;
  error: string | null;
  notFound: boolean;
}

export function useTaskDetail(taskId: number) {
  const [state, setState] = useState<State>({ task: null, loading: true, error: null, notFound: false });
  const latest = useRef(0);

  const refetch = useCallback(async () => {
    const requestId = ++latest.current;
    setState({ task: null, loading: true, error: null, notFound: false });
    try {
      const { task } = await getTask(taskId);
      if (requestId !== latest.current) return;
      setState({ task, loading: false, error: null, notFound: false });
    } catch (err) {
      if (requestId !== latest.current) return;
      const notFound = axios.isAxiosError(err) && err.response?.status === 404;
      setState({
        task: null,
        loading: false,
        notFound,
        error: notFound ? null : err instanceof Error ? err.message : "Failed to load task",
      });
    }
  }, [taskId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  // Lets the drawer apply a saved edit without refetching.
  const setTask = useCallback((next: TaskDetail) => setState((s) => ({ ...s, task: next })), []);

  return { ...state, refetch, setTask };
}