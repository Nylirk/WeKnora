<template>
  <div class="route-registry">
    <div class="filter-bar">
      <t-select
        v-model="filterMethod"
        :placeholder="$t('system.debug.routes.method')"
        clearable
        class="filter-select"
        style="width: 120px"
      >
        <t-option value="GET" label="GET" />
        <t-option value="POST" label="POST" />
        <t-option value="PUT" label="PUT" />
        <t-option value="PATCH" label="PATCH" />
        <t-option value="DELETE" label="DELETE" />
        <t-option value="HEAD" label="HEAD" />
        <t-option value="OPTIONS" label="OPTIONS" />
      </t-select>
      <t-input
        v-model="filterPath"
        :placeholder="$t('system.debug.routes.path')"
        clearable
        class="filter-input"
        style="width: 240px"
      />
      <t-select
        v-model="filterModule"
        :placeholder="$t('system.debug.routes.module')"
        clearable
        class="filter-select"
        style="width: 140px"
      >
        <t-option v-for="m in modules" :key="m" :value="m" :label="m" />
      </t-select>
    </div>

    <t-table
      :data="filteredRoutes"
      :columns="columns"
      row-key="path"
      :loading="loading"
      :empty="$t('system.debug.errors.loadFailed')"
      stripe
      hover
      max-height="460"
      class="route-table"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listDebugRoutes, type DebugRoute } from '@/api/system/debug'

const routes = ref<DebugRoute[]>([])
const loading = ref(false)
const filterMethod = ref('')
const filterPath = ref('')
const filterModule = ref('')

const columns = [
  { colKey: 'method', title: 'Method', width: 90 },
  { colKey: 'path', title: 'Path', ellipsis: true },
  { colKey: 'module', title: 'Module', width: 110 },
  { colKey: 'handler', title: 'Handler', ellipsis: true, width: 200 },
  { colKey: 'auth_required', title: 'Auth', width: 70 },
  { colKey: 'system_admin_required', title: 'System Admin', width: 110 },
]

const modules = computed(() => {
  const set = new Set(routes.value.map((r) => r.module))
  return [...set].sort()
})

const filteredRoutes = computed(() => {
  return routes.value.filter((r) => {
    if (filterMethod.value && r.method !== filterMethod.value) return false
    if (filterPath.value && !r.path.toLowerCase().includes(filterPath.value.toLowerCase())) return false
    if (filterModule.value && r.module !== filterModule.value) return false
    return true
  })
})

onMounted(async () => {
  loading.value = true
  try {
    const res = await listDebugRoutes()
    routes.value = res.routes || []
  } catch {
    routes.value = []
  } finally {
    loading.value = false
  }
})
</script>

<style lang="less" scoped>
.route-registry {
  padding-top: 8px;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.route-table {
  :deep(td[data-col-key="auth_required"]),
  :deep(td[data-col-key="system_admin_required"]) {
    text-align: center;
  }
}
</style>
