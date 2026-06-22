<template>
  <t-drawer
    :visible="visible"
    :header="$t('system.debug.traces.detailTitle')"
    size="560px"
    :footer="false"
    @close="close"
  >
    <div v-if="trace" class="trace-detail">
      <!-- Metadata -->
      <div class="detail-section">
        <div class="section-title">Metadata</div>
        <div class="kv-grid">
          <div class="kv-row"><span class="k">ID</span><span class="v">{{ trace.id }}</span></div>
          <div class="kv-row"><span class="k">Time</span><span class="v">{{ trace.started_at }}</span></div>
          <div class="kv-row"><span class="k">Duration</span><span class="v">{{ trace.duration_ms }}ms</span></div>
          <div class="kv-row"><span class="k">Method</span><span class="v">{{ trace.method }}</span></div>
          <div class="kv-row"><span class="k">Path</span><span class="v">{{ trace.path }}</span></div>
          <div class="kv-row" v-if="trace.query"><span class="k">Query</span><span class="v">{{ trace.query }}</span></div>
          <div class="kv-row"><span class="k">Status</span><span class="v">
            <t-tag :theme="statusTheme(trace.status)" size="small">{{ trace.status }}</t-tag>
          </span></div>
          <div class="kv-row" v-if="trace.user_id"><span class="k">User</span><span class="v">{{ trace.user_id }}</span></div>
          <div class="kv-row" v-if="trace.tenant_id"><span class="k">Tenant</span><span class="v">{{ trace.tenant_id }}</span></div>
          <div class="kv-row" v-if="trace.tenant_role"><span class="k">Role</span><span class="v">{{ trace.tenant_role }}</span></div>
          <div class="kv-row" v-if="trace.is_system_admin"><span class="k">System Admin</span><span class="v">Yes</span></div>
          <div class="kv-row" v-if="trace.error"><span class="k">Error</span><span class="v error-text">{{ trace.error }}</span></div>
        </div>
      </div>

      <!-- Request Headers -->
      <div class="detail-section" v-if="trace.request_headers && Object.keys(trace.request_headers).length">
        <div class="section-title">Request Headers</div>
        <div class="kv-grid">
          <div class="kv-row" v-for="(val, key) in trace.request_headers" :key="'rh-' + key">
            <span class="k">{{ key }}</span>
            <span class="v" :class="{ redacted: val === '[REDACTED]' }">{{ val }}</span>
          </div>
        </div>
      </div>

      <!-- Response Headers -->
      <div class="detail-section" v-if="trace.response_headers && Object.keys(trace.response_headers).length">
        <div class="section-title">Response Headers</div>
        <div class="kv-grid">
          <div class="kv-row" v-for="(val, key) in trace.response_headers" :key="'rsh-' + key">
            <span class="k">{{ key }}</span>
            <span class="v" :class="{ redacted: val === '[REDACTED]' }">{{ val }}</span>
          </div>
        </div>
      </div>

      <!-- Request Body -->
      <div class="detail-section">
        <div class="section-title">
          Request Body
          <t-tag v-if="trace.request_body_truncated" theme="warning" size="small" class="trunc-tag">
            {{ $t('system.debug.traces.truncated') }}
          </t-tag>
        </div>
        <div v-if="!captureBodyEnabled" class="body-disabled">
          {{ $t('system.debug.traces.bodyCaptureDisabled') }}
        </div>
        <pre v-else-if="trace.request_body_preview" class="body-preview">{{ trace.request_body_preview }}</pre>
        <div v-else class="body-empty">—</div>
      </div>

      <!-- Response Body -->
      <div class="detail-section">
        <div class="section-title">
          Response Body
          <t-tag v-if="trace.response_body_truncated" theme="warning" size="small" class="trunc-tag">
            {{ $t('system.debug.traces.truncated') }}
          </t-tag>
        </div>
        <div v-if="!captureBodyEnabled" class="body-disabled">
          {{ $t('system.debug.traces.bodyCaptureDisabled') }}
        </div>
        <pre v-else-if="trace.response_body_preview" class="body-preview">{{ trace.response_body_preview }}</pre>
        <div v-else class="body-empty">—</div>
      </div>
    </div>
    <div v-else class="trace-empty">No trace selected.</div>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { HTTPDebugTrace } from '@/api/system/debug'

const props = defineProps<{
  visible: boolean
  trace: HTTPDebugTrace | null
  captureBodyEnabled: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

function close() {
  emit('update:visible', false)
}

function statusTheme(status: number): string {
  if (status >= 500) return 'danger'
  if (status >= 400) return 'warning'
  if (status >= 200) return 'success'
  return 'default'
}
</script>

<style lang="less" scoped>
.trace-detail {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.detail-section {
  .section-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-secondary);
    margin-bottom: 8px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.kv-grid {
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  padding: 8px 12px;
}

.kv-row {
  display: flex;
  padding: 3px 0;
  font-size: 13px;
  line-height: 1.6;

  .k {
    width: 130px;
    flex-shrink: 0;
    color: var(--td-text-color-placeholder);
    font-weight: 500;
  }

  .v {
    color: var(--td-text-color-primary);
    word-break: break-all;
  }

  .v.redacted {
    color: var(--td-warning-color);
    font-style: italic;
  }
}

.error-text {
  color: var(--td-error-color) !important;
}

.body-preview {
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  padding: 12px;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 300px;
  overflow-y: auto;
  margin: 0;
}

.body-disabled {
  color: var(--td-text-color-placeholder);
  font-style: italic;
  padding: 8px 0;
  font-size: 13px;
}

.body-empty {
  color: var(--td-text-color-placeholder);
  padding: 4px 0;
}

.trunc-tag {
  margin-left: 4px;
}

.trace-empty {
  color: var(--td-text-color-placeholder);
  text-align: center;
  padding: 40px 0;
}
</style>
