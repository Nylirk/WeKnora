import { get, del } from '@/utils/request'

export interface DebugRoute {
  method: string
  path: string
  handler: string
  module: string
  auth_required: boolean
  system_admin_required: boolean
}

export interface HTTPDebugTrace {
  id: string
  started_at: string
  completed_at: string
  duration_ms: number
  method: string
  path: string
  raw_path?: string
  query?: string
  status: number
  user_id?: string
  tenant_id?: number
  tenant_role?: string
  is_system_admin: boolean
  request_content_type?: string
  response_content_type?: string
  request_headers?: Record<string, string>
  response_headers?: Record<string, string>
  request_body_preview?: string
  response_body_preview?: string
  request_body_truncated: boolean
  response_body_truncated: boolean
  error?: string
}

export interface ListDebugRoutesResponse {
  code: number
  msg: string
  routes: DebugRoute[]
}

export interface ListHTTPDebugTracesResponse {
  code: number
  msg: string
  traces: HTTPDebugTrace[]
}

export interface GetHTTPDebugTraceResponse {
  code: number
  msg: string
  trace: HTTPDebugTrace
}

export interface ListHTTPDebugTracesParams {
  /** Filter: path contains this substring. */
  path?: string
  /** Filter: only traces with status >= this value. */
  status_min?: number
  /** Filter: only traces with duration_ms >= this value. */
  slow_ms?: number
}

export async function listDebugRoutes(): Promise<ListDebugRoutesResponse> {
  return (await get('/api/v1/system/admin/debug/routes')) as unknown as ListDebugRoutesResponse
}

export async function listHTTPDebugTraces(
  params?: ListHTTPDebugTracesParams,
): Promise<ListHTTPDebugTracesResponse> {
  const qs = new URLSearchParams()
  if (params?.path) qs.set('path', params.path)
  if (params?.status_min != null) qs.set('status_min', String(params.status_min))
  if (params?.slow_ms != null) qs.set('slow_ms', String(params.slow_ms))
  const tail = qs.toString()
  const url = `/api/v1/system/admin/debug/http-traces${tail ? '?' + tail : ''}`
  return (await get(url)) as unknown as ListHTTPDebugTracesResponse
}

export async function getHTTPDebugTrace(id: string): Promise<GetHTTPDebugTraceResponse> {
  return (await get(`/api/v1/system/admin/debug/http-traces/${encodeURIComponent(id)}`)) as unknown as GetHTTPDebugTraceResponse
}

export async function clearHTTPDebugTraces(): Promise<void> {
  await del('/api/v1/system/admin/debug/http-traces')
}
