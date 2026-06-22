<template>
  <div class="http-trace-panel">
    <!-- Status bar -->
    <div class="status-bar">
      <div class="status-item">
        <span class="status-label">{{ $t('system.debug.traces.enabled') }}</span>
        <t-tag :theme="settingsEnabled ? 'success' : 'default'" size="small">
          {{ settingsEnabled ? 'ON' : 'OFF' }}
        </t-tag>
      </div>
      <div class="status-item">
        <span class="status-label">{{ $t('system.debug.traces.captureBody') }}</span>
        <t-tag :theme="settingsCaptureBody ? 'warning' : 'default'" size="small">
          {{ settingsCaptureBody ? 'ON' : 'OFF' }}
        </t-tag>
      </div>
      <div class="status-item">
        <span class="status-label">max_entries</span>
        <span class="status-value">{{ settingsMaxEntries }}</span>
      </div>
      <div class="status-item">
        <span class="status-label">max_body_bytes</span>
        <span class="status-value">{{ settingsMaxBodyBytes }}</span>
      </div>
      <div class="status-item">
        <span class="status-label">ttl_minutes</span>
        <span class="status-value">{{ settingsTTLMinutes }}</span>
      </div>
    </div>

    <!-- Action bar -->
    <div class="action-bar">
      <div class="filter-group">
        <t-input
          v-model="filterPath"
          :placeholder="$t('system.debug.routes.path')"
          clearable
          style="width: 200px"
        />
        <t-checkbox v-model="filterStatus400">
          {{ $t('system.debug.traces.statusMin400') }}
        </t-checkbox>
        <t-checkbox v-model="filterStatus500">
          {{ $t('system.debug.traces.statusMin500') }}
        </t-checkbox>
        <t-checkbox v-model="filterSlow">
          {{ $t('system.debug.traces.slowRequest') }} (&gt;{{ slowThreshold }}ms)
        </t-checkbox>
      </div>
      <div class="action-group">
        <t-button variant="outline" size="small" @click="refresh" :loading="loading">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
        <t-popconfirm :content="$t('system.debug.traces.clearConfirm')" @confirm="clearTraces">
          <t-button variant="outline" theme="danger" size="small">
            {{ $t('system.debug.traces.clear') }}
          </t-button>
        </t-popconfirm>
      </div>
    </div>

    <!-- Trace table -->
    <t-table
      :data="filteredTraces"
      :columns="columns"
      row-key="id"
      :loading="loading"
      stripe
      hover
      max-height="380"
      @row-click="openDetail"
      class="trace-table"
    />

    <!-- Detail drawer -->
    <HttpTraceDetailDrawer
      v-model:visible="drawerVisible"
      :trace="selectedTrace"
      :capture-body-enabled="settingsCaptureBody"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  listHTTPDebugTraces,
  clearHTTPDebugTraces,
  type HTTPDebugTrace,
} from '@/api/system/debug'
import { listSystemSettings, type SystemSettingItem } from '@/api/system'
import HttpTraceDetailDrawer from './HttpTraceDetailDrawer.vue'

const traces = ref<HTTPDebugTrace[]>([])
const loading = ref(false)
const drawerVisible = ref(false)
const selectedTrace = ref<HTTPDebugTrace | null>(null)

// Settings
const settingsEnabled = ref(false)
const settingsCaptureBody = ref(false)
const settingsMaxEntries = ref(500)
const settingsMaxBodyBytes = ref(4096)
const settingsTTLMinutes = ref(60)

// Filters
const filterPath = ref('')
const filterStatus400 = ref(false)
const filterStatus500 = ref(false)
const filterSlow = ref(false)
const slowThreshold = 1000

const columns = [
  { colKey: 'started_at', title: 'Time', width: 160, cell: 'started_at' },
  { colKey: 'method', title: 'Method', width: 70 },
  { colKey: 'path', title: 'Path', ellipsis: true },
  { colKey: 'status', title: 'Status', width: 70 },
  { colKey: 'duration_ms', title: 'Duration', width: 85 },
  { colKey: 'tenant_id', title: 'Tenant', width: 75 },
  { colKey: 'user_id', title: 'User', width: 100, ellipsis: true },
  { colKey: 'tenant_role', title: 'Role', width: 80 },
]

const filteredTraces = computed(() => {
  return traces.value.filter((t) => {
    if (filterPath.value && !t.path.toLowerCase().includes(filterPath.value.toLowerCase())) return false
    if (filterStatus400.value && t.status < 400) return false
    if (filterStatus500.value && t.status < 500) return false
    if (filterSlow.value && t.duration_ms < slowThreshold) return false
    return true
  })
})

async function loadSettings() {
  try {
    const rows = await listSystemSettings()
    const map = new Map<string, SystemSettingItem>()
    for (const r of rows) map.set(r.key, r)
    settingsEnabled.value = (map.get('debug.http_trace.enabled')?.value as boolean) ?? false
    settingsCaptureBody.value = (map.get('debug.http_trace.capture_body')?.value as boolean) ?? false
    settingsMaxEntries.value = (map.get('debug.http_trace.max_entries')?.value as number) ?? 500
    settingsMaxBodyBytes.value = (map.get('debug.http_trace.max_body_bytes')?.value as number) ?? 4096
    settingsTTLMinutes.value = (map.get('debug.http_trace.ttl_minutes')?.value as number) ?? 60
  } catch {
    // Settings not available — keep defaults
  }
}

async function refresh() {
  loading.value = true
  try {
    await loadSettings()
    const res = await listHTTPDebugTraces()
    traces.value = res.traces || []
  } catch {
    traces.value = []
  } finally {
    loading.value = false
  }
}

async function clearTraces() {
  try {
    await clearHTTPDebugTraces()
    traces.value = []
  } catch {
    // best-effort
  }
}

function openDetail({ row }: { row: HTTPDebugTrace }) {
  selectedTrace.value = row
  drawerVisible.value = true
}

onMounted(() => {
  refresh()
})
</script>

<style lang="less" scoped>
.http-trace-panel {
  padding-top: 8px;
}

.status-bar {
  display: flex;
  gap: 20px;
  padding: 10px 14px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.status-label {
  color: var(--td-text-color-secondary);
}

.status-value {
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 14px;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.action-group {
  display: flex;
  gap: 8px;
}

.trace-table {
  cursor: pointer;
}
</style>
