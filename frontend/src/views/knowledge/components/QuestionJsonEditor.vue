<template>
  <div class="question-json-editor">
    <t-textarea
      :value="modelValue"
      @change="handleChange"
      :autosize="{ minRows: 3, maxRows: 10 }"
      placeholder="JSON"
    />
    <t-button variant="text" size="small" @click="formatJson" style="margin-top: 4px">格式化</t-button>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: string] }>()

function handleChange(val: string) {
  emit('update:modelValue', val)
}

function formatJson() {
  try {
    const parsed = JSON.parse(props.modelValue || '{}')
    emit('update:modelValue', JSON.stringify(parsed, null, 2))
  } catch {
    // leave as-is if invalid
  }
}
</script>

<style scoped>
.question-json-editor { width: 100%; }
</style>