<template>
  <div class="debug-console">
    <div class="section-header">
      <h2>{{ $t('system.debug.title') }}</h2>
      <p class="section-description">{{ $t('system.debug.description') }}</p>
    </div>

    <t-tabs v-model="activeTab" class="debug-tabs">
      <t-tab-panel value="routes" :label="$t('system.debug.tabs.routes')">
        <RouteRegistryPanel />
      </t-tab-panel>
      <t-tab-panel value="traces" :label="$t('system.debug.tabs.httpTraces')">
        <HttpTracePanel />
      </t-tab-panel>
    </t-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import RouteRegistryPanel from './debug/RouteRegistryPanel.vue'
import HttpTracePanel from './debug/HttpTracePanel.vue'

const activeTab = ref('routes')
</script>

<style lang="less" scoped>
.debug-console {
  animation: fadeIn 0.3s ease;
}

.debug-tabs {
  margin-top: 16px;
}
</style>
