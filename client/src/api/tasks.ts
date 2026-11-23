import api from "../axios/api";
import type { ProjectTask, TaskDetail, TaskStatus, UpdateTaskInput } from "../types/task";

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

export async function getTask(taskId: number): Promise<{ message: string; task: TaskDetail }> {
  const { data } = await api.get(`/tasks/${taskId}`);
  return data;
}

export async function updateTask(
  taskId: number,
  input: UpdateTaskInput
): Promise<{ message: string; task: ProjectTask }> {
  const { data } = await api.put(`/tasks/${taskId}`, input);
  return data;
}

export async function deleteTask(taskId: number): Promise<{ message: string }> {
  const { data } = await api.delete(`/tasks/${taskId}`);
  return data;
}