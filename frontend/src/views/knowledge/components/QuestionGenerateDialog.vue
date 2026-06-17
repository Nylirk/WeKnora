<template>
  <t-dialog v-model:visible="dialogVisible" :header="$t('questionBank.generateTitle', '生成题目')" :width="500" @confirm="doGenerate" @close="dialogVisible = false">
    <t-form label-align="top">
      <t-form-item :label="$t('questionBank.name', '题库名称')" :required="true">
        <t-input v-model="name" :placeholder="$t('questionBank.setNamePlaceholder', '请输入题库名称')" />
      </t-form-item>
      <t-form-item :label="$t('questionBank.description', '描述')">
        <t-textarea v-model="description" :placeholder="$t('questionBank.descPlaceholder', '可选描述')" />
      </t-form-item>
      <t-form-item :label="$t('questionBank.generateConfig', '生成配置 (JSON)')">
        <t-textarea v-model="genConfig" :autosize="{ minRows: 3, maxRows: 8 }" placeholder='{"question_count": 20, "question_types": ["single_choice", "short_answer"]}' />
      </t-form-item>
    </t-form>
    <t-alert theme="info" :close-btn="false" style="margin-top: 8px">
      生成功能为预览占位，实际 LLM 生成将在后续版本实现。
    </t-alert>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { generateQuestions } from '@/api/question'

const props = defineProps<{ visible: boolean; knowledgeBaseId: string }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; generated: [] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})
const name = ref('')
const description = ref('')
const genConfig = ref('{}')

async function doGenerate() {
  if (!name.value.trim()) {
    MessagePlugin.warning('请输入题库名称')
    return
  }
  try {
    let config = {}
    try { config = JSON.parse(genConfig.value) } catch { /* use empty */ }
    await generateQuestions(props.knowledgeBaseId, {
      name: name.value.trim(),
      description: description.value.trim(),
      knowledge_base_id: props.knowledgeBaseId,
      generation_config: config,
    })
    MessagePlugin.success('题库创建成功（生成功能将在后续版本实现）')
    dialogVisible.value = false
    emit('generated')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建失败')
  }
}
</script>