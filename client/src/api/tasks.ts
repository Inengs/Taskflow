import api from "../axios/api";
import type { TaskStatus } from "../types/task";

export async function updateTaskStatus(
  taskId: number,
  input: { status: TaskStatus; position: number }
): Promise<{
  message: string;
  task: { id: number; status: TaskStatus; position: number; updated_at: string | null };
}> {
  const { data } = await api.patch(`/tasks/${taskId}/status`, input);
  return data;
}