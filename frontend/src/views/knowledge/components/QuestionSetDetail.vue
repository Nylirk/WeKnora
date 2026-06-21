<template>
  <div class="question-set-detail">
    <div class="detail-header">
      <h2>{{ displaySetName }}</h2>
      <div class="header-actions">

        <t-tooltip v-if="processingButton.state !== 'hidden'" :content="processingButtonTooltip" placement="bottom-right">
          <t-button
            :theme="processingButtonTheme"
            variant="outline"
            shape="round"
            @click="processingDrawerVisible = true"
          >
            <template #icon>
              <t-loading
                v-if="processingButton.state === 'running' || processingButton.state === 'paused'"
                size="small"
                :class="{ 'qp-loading-warning': processingButton.state === 'paused' }"
              />
              <t-icon v-else :name="processingButtonIcon" />
            </template>
            {{ processingButtonLabel }}
          </t-button>
        </t-tooltip>

        <t-popup
          v-model:visible="headerImportMenuVisible"
          trigger="click"
          placement="bottom-right"
          overlay-class-name="question-import-type-popup"
        >
          <t-button theme="primary">{{ $t('questionBank.import') }}</t-button>
          <template #content>
            <div class="import-type-menu">
              <button type="button" class="import-type-item" @click="openManualImport">
                <span class="import-type-title">手动导入</span>
                <span class="import-type-description">手动创建一道题目</span>
              </button>
              <button type="button" class="import-type-item" @click="openSingleImport">
                <span class="import-type-title">文件导入</span>
                <span class="import-type-description">导入一个文件并进入题目审核工作台</span>
              </button>
              <button type="button" class="import-type-item" disabled>
                <span class="import-type-title">批量导入</span>
                <span class="import-type-description">即将支持</span>
              </button>
            </div>
          </template>
        </t-popup>

      </div>
    </div>

    <!-- Processing progress Drawer — waterfall timeline style -->
    <t-drawer
      v-model:visible="processingDrawerVisible"
      :z-index="2100"
      :size="`${qpDrawerWidth}px`"
      attach="body"
      :close-btn="false"
      :footer="false"
      :header="false"
      :show-overlay="true"
      :close-on-overlay-click="true"
      placement="right"
      :class="['kp-secondary-drawer', { 'kp-secondary-drawer--resizing': qpDrawerResizing }]"
    >
      <div class="kp-drawer-shell" :class="{ 'kp-drawer-shell--resizing': qpDrawerResizing }">
        <div class="kp-timeline">
          <div class="kp-shell">
            <!-- HEADER -->
            <div class="kp-head">
              <div class="kp-head-toolbar">
                <h2 class="kp-head-doc-title" :title="displaySetName">{{ displaySetName }}</h2>
                <t-tag v-if="qpHeaderStatusText" size="small" :theme="qpHeaderStatusTheme" variant="light" class="kp-head-status-tag">
                  {{ qpHeaderStatusText }}
                </t-tag>
                <div class="kp-head-actions">
                  <t-popup
                    v-model:visible="reprocessMenuVisible"
                    trigger="click"
                    placement="bottom-right"
                    :disabled="processingButton.state === 'running'"
                  >
                    <t-button
                      size="small"
                      variant="outline"
                      theme="default"
                      :disabled="processingButton.state === 'running'"
                      :loading="reprocessLoading"
                    >
                      重新处理
                    </t-button>
                    <template #content>
                      <div class="reprocess-menu">
                        <button type="button" class="reprocess-item" @click="triggerReprocess('all')">
                          重新处理全部
                        </button>
                        <button type="button" class="reprocess-item" @click="triggerReprocess('auto_tagging')">
                          重新匹配知识点
                        </button>
                        <button type="button" class="reprocess-item" @click="triggerReprocess('syllabus_checking')">
                          重新筛选考纲
                        </button>
                      </div>
                    </template>
                  </t-popup>
                  <button type="button" class="kp-icon-btn" :title="'关闭'" @click="processingDrawerVisible = false">
                    <t-icon name="close" size="16px" />
                  </button>
                </div>
              </div>

              <p class="kp-head-meta">
                <span class="kp-head-meta-part">处理流水线</span>
                <span class="kp-head-meta-sep" aria-hidden="true">·</span>
                <span class="kp-head-meta-part">总耗时 {{ qpFormattedTotal }}</span>
                <span class="kp-head-meta-sep" aria-hidden="true">·</span>
                <span class="kp-head-meta-part">已完成阶段 {{ qpCompletedCount }}/{{ qpTotalCount }}</span>
                <span class="kp-head-meta-sep" aria-hidden="true">·</span>
                <span class="kp-head-meta-part">第 1 次尝试</span>
              </p>
            </div>

            <!-- BODY (Waterfall) -->
            <div class="kp-body" :class="{ 'kp-body-with-detail': selectedProcessingRow }">
              <div v-if="qpShowRuler" class="kp-ruler">
                <div class="kp-ruler-spacer-name" />
                <div class="kp-ruler-spacer-meta" />
                <div class="kp-ruler-track">
                  <span v-for="(tick, i) in qpRulerTicks" :key="i" class="kp-tick"
                    :class="{ 'kp-tick-first': i === 0, 'kp-tick-last': i === qpRulerTicks.length - 1 }"
                    :style="{ left: tick.left }">
                    <span class="kp-tick-line" />
                    <span class="kp-tick-label kp-mono">{{ tick.label }}</span>
                  </span>
                </div>
              </div>

              <div class="kp-scroll">
                <div class="kp-rows">
                  <div v-for="row in qpFlatRows" :key="row.key" class="kp-row"
                    :class="{
                      'kp-row-root': row.isRoot,
                      'kp-row-stage': row.isStage,
                      'kp-row-active': selectedProcessingRow?.key === row.key,
                    }"
                    @click="selectProcessingRow(row)">
                    <div class="kp-cell-name">
                      <div class="kp-name-inner" :style="{ paddingLeft: row.depth * 16 + 'px' }">
                        <span class="kp-tree-toggle-spacer" />
                        <span class="kp-status-dot" :class="[`kp-dot-${row.status}`]" />
                        <span class="kp-name-text" :class="{ 'kp-name-root': row.isRoot, 'kp-name-mono': !row.isRoot }">
                          {{ row.label }}
                        </span>
                      </div>
                    </div>

                    <div class="kp-cell-status">
                      <span class="qp-row-status" :class="`qp-status-${row.status}`">
                        {{ qpRowStatusLabel(row) }}
                      </span>
                    </div>

                    <div class="kp-cell-dur kp-mono">
                      {{ qpDurationLabel(row) }}
                    </div>

                    <div class="kp-cell-bar">
                      <div v-if="row.status === 'paused' || row.status === 'pending' || !row.durationMs" class="kp-bar kp-bar-placeholder" />
                      <div v-else class="kp-bar" :class="[`kp-bar-${row.status}`]" :style="qpBarStyle(row)">
                        <span class="kp-bar-tip">
                          <span class="kp-bar-tip-name">{{ row.label }}</span>
                          <span class="kp-bar-tip-sep">·</span>
                          <span class="kp-mono">{{ row.formattedDur }}</span>
                          <span class="kp-bar-tip-sep">·</span>
                          <span>{{ PROCESSING_STAGE_STATUS_LABELS[row.status] || row.status }}</span>
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- DETAIL PANEL -->
            <div class="kp-detail" :class="{ 'kp-detail-open': selectedProcessingRow }">
              <template v-if="selectedProcessingRow">
                <div class="kp-detail-head">
                  <div class="kp-detail-title">
                    <span class="kp-status-dot kp-detail-dot" :class="[`kp-dot-${selectedProcessingRow.status}`]" />
                    <span class="kp-detail-name">{{ selectedProcessingRow.label }}</span>
                    <span class="kp-detail-kind">{{ selectedProcessingRow.kind }}</span>
                    <span class="kp-status-chip" :class="`kp-chip-${selectedProcessingRow.status}`">
                      {{ PROCESSING_STAGE_STATUS_LABELS[selectedProcessingRow.status] || selectedProcessingRow.status }}
                    </span>
                  </div>
                  <div class="kp-detail-actions">
                    <button type="button" class="kp-icon-btn" title="关闭" @click="selectedProcessingRow = null">
                      <t-icon name="close" size="18px" />
                    </button>
                  </div>
                </div>

                <div class="kp-tabs">
                  <button type="button" class="kp-tab" :class="{ 'kp-tab-active': processingDetailTab === 'overview' }"
                    @click="processingDetailTab = 'overview'">概览</button>
                  <button type="button" class="kp-tab" :class="{ 'kp-tab-active': processingDetailTab === 'input' }"
                    @click="processingDetailTab = 'input'">输入</button>
                  <button type="button" class="kp-tab" :class="{ 'kp-tab-active': processingDetailTab === 'output' }"
                    @click="processingDetailTab = 'output'">输出</button>
                  <button type="button" class="kp-tab" :class="{ 'kp-tab-active': processingDetailTab === 'raw' }"
                    @click="processingDetailTab = 'raw'">原始 JSON</button>
                </div>

                <div class="kp-detail-body">
                  <!-- Overview tab -->
                  <template v-if="processingDetailTab === 'overview'">
                    <div class="kp-section">
                      <div class="kp-section-title">状态</div>
                      <div class="kp-kv">
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">状态</span>
                          <span class="kp-kv-val">
                            <span class="kp-status-chip" :class="`kp-chip-${selectedProcessingRow.status}`">
                              {{ PROCESSING_STAGE_STATUS_LABELS[selectedProcessingRow.status] || selectedProcessingRow.status }}
                            </span>
                          </span>
                        </div>
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">耗时</span>
                          <span class="kp-kv-val kp-mono">{{ selectedProcessingRow.formattedDur }}</span>
                        </div>
                        <div v-if="selectedProcessingRow.reason" class="kp-kv-row">
                          <span class="kp-kv-key">原因</span>
                          <span class="kp-kv-val" style="color:var(--td-warning-color)">{{ selectedProcessingRow.reason }}</span>
                        </div>
                      </div>
                    </div>
                    <div class="kp-section">
                      <div class="kp-section-title">身份信息</div>
                      <div class="kp-kv">
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">名称</span>
                          <span class="kp-kv-val">{{ selectedProcessingRow.label }}</span>
                        </div>
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">类型</span>
                          <span class="kp-kv-val kp-mono">{{ selectedProcessingRow.kind.toUpperCase() }}</span>
                        </div>
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">状态</span>
                          <span class="kp-kv-val">{{ PROCESSING_STAGE_STATUS_LABELS[selectedProcessingRow.status] || selectedProcessingRow.status }}</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- Input tab -->
                  <template v-else-if="processingDetailTab === 'input'">
                    <div class="kp-section">
                      <div class="kp-section-title">配置快照</div>
                      <div class="kp-kv">
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">知识点关联</span>
                          <span class="kp-kv-val">{{ processingStatus?.auto_tagging_enabled ? '已启用' : '未启用' }}</span>
                        </div>
                        <div class="kp-kv-row">
                          <span class="kp-kv-key">考纲筛选</span>
                          <span class="kp-kv-val">{{ processingStatus?.syllabus_check_enabled ? '已启用' : '未启用' }}</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- Output tab -->
                  <template v-else-if="processingDetailTab === 'output'">
                    <div class="kp-section">
                      <div class="kp-section-title">阶段结果</div>
                      <div class="kp-kv">
                        <div class="kp-kv-row" v-if="processingStatus?.skipped_auto_tagging_reason">
                          <span class="kp-kv-key">知识点关联</span>
                          <span class="kp-kv-val" style="color:var(--td-warning-color)">{{ processingStatus.skipped_auto_tagging_reason }}</span>
                        </div>
                        <div class="kp-kv-row" v-if="processingStatus?.skipped_syllabus_reason">
                          <span class="kp-kv-key">考纲筛选</span>
                          <span class="kp-kv-val" style="color:var(--td-warning-color)">{{ processingStatus.skipped_syllabus_reason }}</span>
                        </div>
                        <div v-if="!processingStatus?.skipped_auto_tagging_reason && !processingStatus?.skipped_syllabus_reason" class="kp-kv-row">
                          <span class="kp-kv-val">所有阶段正常运行</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- Raw JSON tab -->
                  <template v-else-if="processingDetailTab === 'raw'">
                    <div class="kp-section">
                      <pre class="kp-json kp-mono">{{ qpRowJson(selectedProcessingRow) }}</pre>
                    </div>
                  </template>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </t-drawer>


    <div class="filter-bar">
      <t-select v-model="filter.question_type" :placeholder="$t('questionBank.typeFilter', '题型')" clearable style="width: 120px" @change="reloadFromFirstPage">
        <t-option v-for="qt in questionTypes" :key="qt" :value="qt" :label="questionTypeLabel(qt)" />
      </t-select>
      <t-select v-model="filter.status" :placeholder="$t('questionBank.statusFilter', '状态')" clearable style="width: 100px" @change="reloadFromFirstPage">
        <t-option value="draft" :label="$t('questionBank.draft', '草稿')" />
        <t-option value="reviewed" :label="$t('questionBank.reviewed', '已审')" />
        <t-option value="rejected" :label="$t('questionBank.rejected', '已拒')" />
      </t-select>
      <t-select v-model="filter.difficulty" :placeholder="$t('questionBank.difficultyFilter', '难度')" clearable style="width: 100px" @change="reloadFromFirstPage">
        <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
        <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
        <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
      </t-select>
      <t-select v-model="filter.auto_tagging_status" placeholder="知识点匹配" clearable style="width: 120px" @change="reloadFromFirstPage">
        <t-option value="" label="全部" />
        <t-option value="matched" label="已匹配" />
        <t-option value="unmatched" label="未匹配" />
        <t-option value="paused" label="暂停" />
        <t-option value="failed" label="失败" />
        <t-option value="pending" label="待处理" />
      </t-select>
      <t-select v-model="syllabusFilterValue" placeholder="考纲筛选" clearable style="width: 120px" @change="onSyllabusFilterChange">
        <t-option value="" label="全部" />
        <t-option value="scope:in_scope" label="符合考纲" />
        <t-option value="scope:out_of_scope" label="疑似超纲" />
        <t-option value="scope:uncertain" label="不确定" />
        <t-option value="status:paused" label="暂停" />
        <t-option value="status:failed" label="失败" />
        <t-option value="status:pending" label="待处理" />
      </t-select>
      <t-input v-model="filter.knowledge_point" placeholder="知识点" clearable style="width: 140px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
      <t-input v-model="filter.tag" placeholder="标签" clearable style="width: 120px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
      <t-input v-model="filter.keyword" :placeholder="$t('questionBank.searchPlaceholder', '搜索题干...')" clearable style="width: 180px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
    </div>

    <!-- Batch action bar -->
    <div v-if="selectedRowKeys.length" class="batch-actions">
      <span class="batch-label">已选择 {{ selectedRowKeys.length }} 题</span>
      <t-button size="small" variant="outline" @click="batchReview">批量审核</t-button>
      <t-popconfirm content="确定要删除选中题目？此操作不可撤销。" @confirm="batchDelete">
        <t-button size="small" variant="outline" theme="danger">批量删除</t-button>
      </t-popconfirm>
      <t-button size="small" variant="text" @click="selectedRowKeys = []">清空选择</t-button>
    </div>

    <t-table
      v-if="loading || questions.length > 0"
      :data="questions"
      :columns="questionColumns"
      :loading="loading"
      :selected-row-keys="selectedRowKeys"
      :pagination="{ current: currentPage, pageSize, total: questionTotal, showJumper: true, showPageSize: true, pageSizeOptions: [20, 50, 100, 200] }"
      row-key="id"
      hover
      @select-change="onSelectChange"
      @page-change="onPageChange"
    >
      <template #question_type="{ row }">
        {{ questionTypeLabel(row.question_type) }}
      </template>
      <template #stem_text="{ row }">
        <t-popup :placement="semanticPopupPlacement" trigger="hover" show-arrow attach="body">
          <span class="question-stem-cell" @mouseenter="updateSemanticPopupPlacement($event, 260)">{{ row.stem_text }}</span>
          <template #content>
            <div class="semantic-popover semantic-popover-stem">
              <div class="semantic-popover-title">题干</div>
              <div class="semantic-stem-text">{{ row.stem_text || '—' }}</div>
            </div>
          </template>
        </t-popup>
      </template>
      <template #difficulty="{ row }">
        {{ difficultyLabel(row.difficulty) }}
      </template>
      <template #auto_tagging_status="{ row }">
        <template v-if="(row.auto_tagging_status === 'matched' || row.auto_tagging_status === 'completed') && getTopKnowledgePointCandidate(row)">
          <t-popup :placement="semanticPopupPlacement" trigger="hover" show-arrow attach="body">
            <t-tag theme="success" variant="light" size="small" class="question-match-tag" @mouseenter="updateSemanticPopupPlacement($event, 280)">
              <span class="question-match-tag-text">
                {{ getTopKnowledgePointCandidate(row)?.knowledge_point }}
                <template v-if="formatConfidence(getTopKnowledgePointCandidate(row)?.confidence)">
                  · {{ formatConfidence(getTopKnowledgePointCandidate(row)?.confidence) }}
                </template>
              </span>
            </t-tag>

            <template #content>
              <div class="semantic-popover">
                <div class="semantic-popover-title">知识点匹配详情</div>

                <div
                  v-for="(candidate, index) in getKnowledgePointCandidates(row)"
                  :key="`${candidate.knowledge_point}-${index}`"
                  class="semantic-candidate"
                >
                  <div class="semantic-candidate-head">
                    <span class="semantic-candidate-name">{{ candidate.knowledge_point }}</span>
                    <t-tag size="small" theme="success" variant="light">
                      {{ formatConfidence(candidate.confidence) || '—' }}
                    </t-tag>
                  </div>

                  <div class="semantic-meta-row">
                    <span class="semantic-meta-label">分数</span>
                    <span class="semantic-meta-value">
                      {{ typeof candidate.score === 'number' ? candidate.score.toFixed(3) : '—' }}
                    </span>
                  </div>

                </div>
              </div>
            </template>
          </t-popup>
        </template>
        <template v-else-if="row.auto_tagging_status === 'matched' || row.auto_tagging_status === 'completed'">
          <t-tag theme="default" variant="light" size="small">未匹配</t-tag>
        </template>
        <t-tag v-else-if="row.auto_tagging_status === 'unmatched'" theme="default" variant="light" size="small">未匹配</t-tag>
        <t-tag v-else-if="row.auto_tagging_status === 'paused'" theme="warning" variant="light" size="small">暂停</t-tag>
        <t-tag v-else-if="row.auto_tagging_status === 'failed'" theme="danger" variant="light" size="small">失败</t-tag>
        <t-tag v-else theme="default" variant="light" size="small">待处理</t-tag>
      </template>
      <template #syllabus_scope_result="{ row }">
        <template v-if="syllabusDisplayLabel(row) !== '—'">
          <t-popup :placement="semanticPopupPlacement" trigger="hover" show-arrow attach="body">
            <t-tag :theme="syllabusTagTheme(row)" variant="light" size="small" @mouseenter="updateSemanticPopupPlacement($event, 240)">
              {{ syllabusDisplayLabel(row) }}
            </t-tag>

            <template #content>
              <div class="semantic-popover">
                <div class="semantic-popover-title">考纲筛选详情</div>

                <div class="semantic-meta-row">
                  <span class="semantic-meta-label">结果</span>
                  <span class="semantic-meta-value">{{ syllabusDisplayLabel(row) }}</span>
                </div>

                <div v-if="getSyllabusDetail(row).reason" class="semantic-meta-row">
                  <span class="semantic-meta-label">原因</span>
                  <span class="semantic-meta-value">{{ getSyllabusDetail(row).reason }}</span>
                </div>

                <div class="semantic-meta-row">
                  <span class="semantic-meta-label">置信度</span>
                  <span class="semantic-meta-value">{{ formatConfidence(getSyllabusDetail(row).confidence) || '—' }}</span>
                </div>

                <div class="semantic-meta-row">
                  <span class="semantic-meta-label">分数</span>
                  <span class="semantic-meta-value">
                    {{ typeof getSyllabusDetail(row).score === 'number' ? getSyllabusDetail(row).score.toFixed(3) : '—' }}
                  </span>
                </div>

              </div>
            </template>
          </t-popup>
        </template>
        <span v-else class="qp-na">—</span>
      </template>
      <template #status="{ row }">
        <t-tooltip v-if="row.status === 'reviewed' && row.reviewed_at" :content="`审核人：${row.reviewed_by || '未知'}\n审核时间：${row.reviewed_at}`">
          <t-tag theme="success" size="small">{{ statusLabel(row.status) }}</t-tag>
        </t-tooltip>
        <t-link v-else-if="row.status === 'draft'" theme="primary" hover="color" @click="reviewSingleQuestion(row)">
          <t-tag theme="default" size="small" class="draft-review-tag">{{ statusLabel(row.status) }}</t-tag>
        </t-link>
        <t-tag v-else :theme="row.status === 'rejected' ? 'danger' : 'default'" size="small">
          {{ statusLabel(row.status) }}
        </t-tag>
      </template>
      <template #operation="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openEditDialog(row)">{{ $t('common.edit', '编辑') }}</t-link>
          <t-link theme="danger" @click="removeQuestion(row)">{{ $t('common.delete', '删除') }}</t-link>
        </t-space>
      </template>
    </t-table>
    <t-empty v-else description="当前题集暂无题目" class="question-empty" />

    <QuestionEditDialog
      v-model:visible="editVisible"
      :question="editingQuestion"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      @saved="refreshAfterMutation"
    />
    <QuestionFileImportDialog
      :key="fileImportSession"
      v-model:visible="fileImportVisible"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      import-mode="single"
      @parsed="handleFileParsed"
    />
    <QuestionImportWorkbench
      v-model:visible="workbenchVisible"
      :kb-id="knowledgeBaseId"
      :set-id="setId"
      @imported="handleWorkbenchImported"
      @abandoned="handleWorkbenchAbandoned"
    />
    <t-dialog
      v-model:visible="restoreDraftVisible"
      header="发现未完成的导入草稿"
      attach="body"
      :z-index="3200"
      :close-btn="false"
      :close-on-overlay-click="false"
      :close-on-esc-keydown="false"
      :confirm-btn="{ content: '恢复草稿', theme: 'primary' }"
      :cancel-btn="{ content: '重新导入' }"
      @confirm="restoreImportDraft"
      @cancel="startFreshImport"
    >
      <p class="restore-draft-copy">
        该题集存在 7 天内保存的导入草稿（{{ pendingDraftTime }}），是否继续处理？
      </p>
    </t-dialog>

    <!-- P2: Global loading overlay (z-index 6000, above all import dialogs) -->
    <Teleport to="body">
      <div v-if="importUI.visible" class="import-loading-overlay" :class="{ leaving: importUI.leaving }">
        <div class="import-loading-content">
          <t-loading size="medium" />
          <span class="import-loading-text">{{ importUI.loadingText || '处理中…' }}</span>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  getQuestionSet, listQuestions, deleteQuestion as apiDeleteQuestion,
  updateQuestionStatus, getQuestionSetProcessingStatus,
  resolveProcessingStages, resolveProcessingButtonState,
  PROCESSING_STAGE_STATUS_LABELS, PROCESSING_BUTTON_LABELS,
  reprocessQuestionSet,
  type Question, type QuestionListFilter, type QuestionType,
  type QuestionSetProcessingStatus,
  type ProcessingButtonState,
  type QuestionProcessingReprocessScope,
} from '@/api/question'
import type { BlockPreviewSummary, ImportBlock } from '@/api/question_block'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import { useImportUIStore } from '@/stores/importUIStore'
import {
  cleanExpiredDrafts, deleteDraft, loadDraft, saveDraft, type ImportDraft,
} from '@/utils/importDraftDB'
import { resolveQuestionRows, resolveQuestionTotal } from '../questionData'

const props = defineProps<{ setId: string; knowledgeBaseId: string; setName?: string }>()
const emit = defineEmits<{ changed: [total: number] }>()
const workbenchStore = useImportWorkbenchStore()
const importUI = useImportUIStore()

// Processing status
const processingStatus = ref<QuestionSetProcessingStatus | null>(null)
const processingDrawerVisible = ref(false)
let processingPollTimer: ReturnType<typeof setInterval> | null = null

// Derived processing state
const processingStages = computed(() => {
  if (!processingStatus.value) return []
  return resolveProcessingStages(processingStatus.value)
})

const processingButton = computed(() => {
  return resolveProcessingButtonState(processingStatus.value)
})

const processingButtonLabel = computed(() => {
  const btn = processingButton.value
  if (btn.state === 'running') {
    return `${PROCESSING_BUTTON_LABELS[btn.state]} ${btn.completedCount}/${btn.totalCount}`
  }
  return PROCESSING_BUTTON_LABELS[btn.state]
})

const processingButtonTooltip = computed(() => {
  const btn = processingButton.value
  if (btn.state === 'running') {
    return `题目处理中 (${btn.completedCount}/${btn.totalCount})`
  }
  if (btn.state === 'paused') {
    return '部分处理阶段已暂停，点击查看详情'
  }
  if (btn.state === 'failed') {
    return '处理失败，点击查看错误详情'
  }
  if (btn.state === 'ready_for_review') {
    return '自动处理已完成，点击查看进度'
  }
  return '点击查看处理进度'
})

const processingButtonTheme = computed(() => {
  const themeMap: Record<ProcessingButtonState, string> = {
    hidden: 'default',
    running: 'primary',
    paused: 'warning',
    failed: 'danger',
    ready_for_review: 'success',
    completed: 'success',
  }
  return themeMap[processingButton.value.state] || 'default'
})

const processingButtonIcon = computed(() => {
  const iconMap: Record<ProcessingButtonState, string> = {
    hidden: '',
    running: 'loading',
    paused: 'loading',
    failed: 'close-circle',
    ready_for_review: 'check-circle',
    completed: 'check-circle',
  }
  return iconMap[processingButton.value.state] || 'info-circle'
})

// ── Waterfall timeline computed ──
const qpDrawerWidth = ref(820)
const qpDrawerResizing = ref(false)

const STAGE_LABELS: Record<string, string> = {
  draft_imported: '导入完成',
  indexing: '索引处理',
  auto_tagging: '知识点关联',
  syllabus_checking: '考纲筛选',
}

const selectedProcessingRow = ref<QpFlatRow | null>(null)
const processingDetailTab = ref<'overview' | 'input' | 'output' | 'raw'>('overview')

function selectProcessingRow(row: QpFlatRow) {
  if (selectedProcessingRow.value?.key === row.key) {
    selectedProcessingRow.value = null
    return
  }
  selectedProcessingRow.value = row
  processingDetailTab.value = 'overview'
}

function qpRowJson(row: QpFlatRow): string {
  try {
    return JSON.stringify(row, null, 2)
  } catch {
    return String(row)
  }
}

function qpRowStatusLabel(row: QpFlatRow): string {
  if (row.isRoot) {
    switch (processingButton.value.state) {
      case 'paused': return '部分暂停'
      case 'running': return '进行中'
      case 'failed': return '处理失败'
      case 'ready_for_review': return '待人工审核'
      case 'completed': return '已完成'
      default: return '未开始'
    }
  }
  switch (row.status) {
    case 'completed':
    case 'done': return '已完成'
    case 'running': return '进行中'
    case 'paused': return '暂停'
    case 'failed': return '失败'
    case 'pending': return '待处理'
    default: return '未知'
  }
}

function qpDurationLabel(row: QpFlatRow): string {
  return row.formattedDur || '—'
}

interface QpFlatRow {
  key: string
  depth: number
  label: string
  kind: string
  status: string
  reason?: string
  isRoot: boolean
  isStage: boolean
  isPlaceholder: boolean
  durationMs: number
  startMs: number
  formattedDur: string
}

const qpFlatRows = computed<QpFlatRow[]>(() => {
  const stages = processingStages.value
  if (!stages.length) return []

  const rows: QpFlatRow[] = []

  // ROOT row
  rows.push({
    key: '__root__',
    depth: 0,
    label: '题库处理',
    kind: 'root',
    status: processingStatus.value?.stage === 'failed' ? 'failed'
      : processingButton.value.state === 'running' ? 'running'
      : processingButton.value.state === 'ready_for_review' ? 'done'
      : processingButton.value.state === 'completed' ? 'done'
      : processingButton.value.state === 'paused' ? 'paused'
      : 'pending',
    isRoot: true,
    isStage: false,
    isPlaceholder: false,
    durationMs: 0,
    startMs: 0,
    formattedDur: '—',
  })

  stages.forEach((s) => {
    const isPaused = s.status === 'paused'
    const isPending = s.status === 'pending'
    rows.push({
      key: s.key,
      depth: 1,
      label: STAGE_LABELS[s.key] || s.label || s.key,
      kind: 'stage',
      status: s.status,
      reason: s.reason,
      isRoot: false,
      isStage: true,
      isPlaceholder: isPending || isPaused,
      durationMs: 0,
      startMs: 0,
      formattedDur: '—',
    })
  })

  return rows
})

const qpFormattedTotal = computed(() => '—')

const qpShowRuler = computed(() => false)

const qpRulerTicks = computed(() => [])

const qpCompletedCount = computed(() =>
  processingStages.value.filter(s => s.status === 'completed').length,
)

const qpTotalCount = computed(() => processingStages.value.length)

const qpHeaderStatusText = computed(() => {
  if (processingStatus.value?.stage === 'failed') return '处理失败'
  if (processingButton.value.state === 'running') return '进行中'
  if (processingButton.value.state === 'paused') return '部分暂停'
  if (processingButton.value.state === 'ready_for_review') return '待人工审核'
  if (processingButton.value.state === 'completed') return '已完成'
  return ''
})

const qpHeaderStatusTheme = computed(() => {
  if (processingStatus.value?.stage === 'failed') return 'danger'
  if (processingButton.value.state === 'running') return 'warning'
  if (processingButton.value.state === 'paused') return 'warning'
  if (processingButton.value.state === 'ready_for_review' || processingButton.value.state === 'completed') return 'success'
  return 'default'
})

function qpBarStyle(_row: QpFlatRow): Record<string, string> {
  return { display: 'none' }
}

// ── End waterfall timeline ──

async function fetchProcessingStatus() {
  if (!props.knowledgeBaseId || !props.setId) return
  try {
    const response: any = await getQuestionSetProcessingStatus(props.knowledgeBaseId, props.setId)
    processingStatus.value = response?.data ?? response
    if (processingStatus.value) {
      const stage = processingStatus.value.stage
      if (stage === 'ready_for_review' || stage === 'failed' || stage === '') {
        stopProcessingPolling()
      }
    }
  } catch {
    // best-effort
  }
}

function startProcessingPolling() {
  stopProcessingPolling()
  fetchProcessingStatus()
  processingPollTimer = setInterval(fetchProcessingStatus, 5000)
}

function stopProcessingPolling() {
  if (processingPollTimer !== null) {
    clearInterval(processingPollTimer)
    processingPollTimer = null
  }
}

// ── Reprocess trigger ──
const reprocessMenuVisible = ref(false)
const reprocessLoading = ref(false)

async function triggerReprocess(scope: QuestionProcessingReprocessScope) {
  reprocessMenuVisible.value = false
  reprocessLoading.value = true
  try {
    await reprocessQuestionSet(props.knowledgeBaseId, props.setId, scope)
    MessagePlugin.success('重新处理已启动')
    await fetchProcessingStatus()
    startProcessingPolling()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新处理失败')
  } finally {
    reprocessLoading.value = false
  }
}

const questionTypes: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'short_answer', 'essay', 'composite']
const questionColumns = computed(() => [
  { colKey: 'row-select', type: 'multiple' as const, width: 50 },
  { colKey: 'question_type', title: '类型', width: 80, cell: 'question_type' },
  { colKey: 'stem_text', title: '题干', minWidth: 360, ellipsis: true, cell: 'stem_text' },
  { colKey: 'difficulty', title: '难度', width: 72, cell: 'difficulty' },
  { colKey: 'auto_tagging_status', title: '知识点', width: 170, cell: 'auto_tagging_status' },
  { colKey: 'syllabus_scope_result', title: '考纲', width: 100, cell: 'syllabus_scope_result' },
  { colKey: 'status', title: '状态', width: 80, cell: 'status' },
  { colKey: 'operation', title: '操作', width: 120, fixed: 'right', cell: 'operation' },
])
const fetchedSetName = ref('')
const displaySetName = computed(() => props.setName?.trim() || fetchedSetName.value)
const questions = ref<Question[]>([])
const loading = ref(false)
const filter = ref<QuestionListFilter>({})
const editVisible = ref(false)
const fileImportVisible = ref(false)
const fileImportSession = ref(0)
const workbenchVisible = ref(false)
const restoreDraftVisible = ref(false)
const pendingDraft = ref<ImportDraft | null>(null)
const pendingDraftTime = computed(() => pendingDraft.value
  ? new Date(pendingDraft.value.timestamp).toLocaleString()
  : '')
const headerImportMenuVisible = ref(false)
const editingQuestion = ref<Question | null>(null)
const selectedRowKeys = ref<string[]>([])
const currentPage = ref(1)
const pageSize = ref(50)
const questionTotal = ref(0)

function onSelectChange(value: string[]) {
  selectedRowKeys.value = value
}

function onPageChange(pageInfo: { current: number; pageSize: number }) {
  currentPage.value = pageInfo.current
  pageSize.value = pageInfo.pageSize
  selectedRowKeys.value = []
  loadQuestions()
}

function reloadFromFirstPage() {
  currentPage.value = 1
  selectedRowKeys.value = []
  loadQuestions()
}

async function reviewSingleQuestion(row: Question) {
  if (row.status !== 'draft') return
  try {
    await updateQuestionStatus(props.knowledgeBaseId, props.setId, row.id, { status: 'reviewed' })
    MessagePlugin.success('审核成功')
    await refreshAfterMutation()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '审核失败')
  }
}

async function batchReview() {
  const draftIds = selectedRowKeys.value.filter(id => {
    const q = questions.value.find(q => q.id === id)
    return q?.status === 'draft'
  })
  if (!draftIds.length) {
    MessagePlugin.warning('没有可审核的草稿题目')
    return
  }
  let done = 0; let failed = 0
  for (const id of draftIds) {
    try {
      await updateQuestionStatus(props.knowledgeBaseId, props.setId, id, { status: 'reviewed' })
      done++
    } catch { failed++ }
  }
  MessagePlugin.success(`审核完成：成功 ${done} 题` + (failed ? `，失败 ${failed} 题` : ''))
  selectedRowKeys.value = []
  await refreshAfterMutation()
}

async function batchDelete() {
  if (!selectedRowKeys.value.length) return
  let done = 0; let failed = 0
  for (const id of selectedRowKeys.value) {
    try {
      await apiDeleteQuestion(props.knowledgeBaseId, props.setId, id)
      done++
    } catch { failed++ }
  }
  MessagePlugin.success(`删除完成：成功 ${done} 题` + (failed ? `，失败 ${failed} 题` : ''))
  selectedRowKeys.value = []
  await refreshAfterMutation()
}

async function loadQuestions(): Promise<number | null> {
  loading.value = true
  try {
    const res = await listQuestions(props.knowledgeBaseId, props.setId, filter.value, currentPage.value, pageSize.value)
    const rows = resolveQuestionRows<Question>(res)
    const total = resolveQuestionTotal(res, rows)
    questions.value = rows
    questionTotal.value = total
    return total
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载题目失败')
    questions.value = []
    return null
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  editingQuestion.value = null
  editVisible.value = true
}

function openEditDialog(q: Question) {
  editingQuestion.value = q
  editVisible.value = true
}

async function closeAllImportMenus() {
  headerImportMenuVisible.value = false
  await nextTick()
}

function closeImportModals() {
  fileImportVisible.value = false
  restoreDraftVisible.value = false
}

async function openSingleImport() {
  await closeAllImportMenus()
  closeImportModals()
  await nextTick()

  try {
    await cleanExpiredDrafts()
    const draft = await loadDraft(props.knowledgeBaseId, props.setId)
    if (draft) {
      pendingDraft.value = draft
      restoreDraftVisible.value = true
      return
    }
  } catch (error: any) {
    MessagePlugin.warning(error?.message || '读取导入草稿失败，将开始新的导入。')
  }

  await openFileImportDialog()
}

async function openFileImportDialog() {
  closeImportModals()
  await nextTick()

  pendingDraft.value = null
  fileImportSession.value += 1
  fileImportVisible.value = true
}

function applyDraftToWorkbench(draft: ImportDraft) {
  workbenchStore.reset()
  workbenchStore.kbId = props.knowledgeBaseId
  workbenchStore.setId = props.setId
  workbenchStore.loadFromDraft(draft)
}

async function restoreImportDraft() {
  await importUI.withImportLoading('正在恢复草稿…', async () => {
    const draft = pendingDraft.value
    const hasBlocks = (Array.isArray(draft.blocks) && draft.blocks.length > 0) || (Array.isArray(draft.blockOrder) && draft.blockOrder.length > 0)
    if (!draft || !hasBlocks) {
      MessagePlugin.warning('草稿中没有可恢复的 blocks，请重新导入。')
      await startFreshImport()
      return
    }
    fileImportVisible.value = false
    restoreDraftVisible.value = false
    headerImportMenuVisible.value = false
    applyDraftToWorkbench(draft)
    pendingDraft.value = null
    await nextTick()
    workbenchVisible.value = true
  })
}

async function startFreshImport() {
  await importUI.withImportLoading('正在重新导入…', async () => {
    closeImportModals()
    pendingDraft.value = null
    await deleteDraft(props.knowledgeBaseId, props.setId)
    restoreDraftVisible.value = false
    await nextTick()
    await openFileImportDialog()
  })
}

async function handleFileParsed(payload: {
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
  strategyPreset: string
  importFormat: 'json' | 'word' | 'pdf'
  importMode: 'single' | 'batch'
}) {
  try {
    fileImportVisible.value = false
    restoreDraftVisible.value = false
    headerImportMenuVisible.value = false
    pendingDraft.value = null
    workbenchStore.reset()
    workbenchStore.kbId = props.knowledgeBaseId
    workbenchStore.setId = props.setId
    workbenchStore.strategyPreset = payload.strategyPreset
    workbenchStore.defaultDifficulty = 'medium'
    workbenchStore.importMode = payload.importMode
    workbenchStore.importFormat = payload.importFormat
    workbenchStore.setBlocksFromResponse(payload.blocks)

    try {
      const blockOrder = payload.blocks.map(b => b.id)
      const blockMap: Record<string, ImportBlock> = {}
      for (const b of payload.blocks) { blockMap[b.id] = b }
      await saveDraft({
        kbId: props.knowledgeBaseId,
        setId: props.setId,
        blockOrder,
        blockMap,
        deletedBlockStack: [],
        deletedBlockMap: {},
        strategyPreset: payload.strategyPreset,
        defaultDifficulty: workbenchStore.defaultDifficulty,
        importMode: payload.importMode,
        importFormat: payload.importFormat,
        currentStep: 'block-review',
        questions: [],
        timestamp: Date.now(),
      })
    } catch (error: any) {
      MessagePlugin.warning(error?.message || '草稿保存失败，本次仍可继续处理。')
    }

    await nextTick()
    workbenchVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打开导入工作台失败')
    console.error('[question-import] failed to open workbench', e)
  }
}

async function handleWorkbenchImported() {
  workbenchVisible.value = false
  await refreshAfterMutation()
  // Restart polling to pick up new processing status
  startProcessingPolling()
}

function handleWorkbenchAbandoned() {
  workbenchVisible.value = false
}

function openManualImport() {
  headerImportMenuVisible.value = false
  openCreateDialog()
}

async function refreshAfterMutation() {
  selectedRowKeys.value = []
  const total = await loadQuestions()
  // If current page is empty and past page 1, go back one page
  if (total !== null && questions.value.length === 0 && currentPage.value > 1) {
    currentPage.value -= 1
    await loadQuestions()
  }
  if (total !== null) emit('changed', total)
}

async function removeQuestion(q: Question) {
  try {
    await apiDeleteQuestion(props.knowledgeBaseId, props.setId, q.id)
    MessagePlugin.success('删除成功')
    await refreshAfterMutation()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

function questionTypeLabel(t: QuestionType) {
  const map: Record<QuestionType, string> = {
    single_choice: '单选', multiple_choice: '多选', true_false: '判断',
    fill_blank: '填空', short_answer: '简答', essay: '论述', composite: '复合',
  }
  return map[t] || t
}
function difficultyLabel(d: string) {
  const map: Record<string, string> = { easy: '简单', medium: '中等', hard: '困难' }
  return map[d] || d
}
function statusLabel(s: string) {
  const map: Record<string, string> = { draft: '草稿', reviewed: '已审', rejected: '已拒' }
  return map[s] || s
}

function getKnowledgePointCandidates(row: Question): Array<{
  knowledge_point: string
  confidence?: number
  score?: number
}> {
  const candidates = (row.extraction_metadata as any)?.auto_processing?.auto_tagging?.candidates
  if (!Array.isArray(candidates)) return []

  return candidates
    .filter((c: any) => c && typeof c === 'object')
    .slice(0, 3)
    .map((c: any) => ({
      knowledge_point: String(c.knowledge_point || '未知知识点'),
      confidence: typeof c.confidence === 'number' ? c.confidence : undefined,
      score: typeof c.score === 'number' ? c.score : undefined,
    }))
}

function getTopKnowledgePointCandidate(row: Question): {
  knowledge_point: string
  confidence?: number
  score?: number
} | null {
  return getKnowledgePointCandidates(row)[0] || null
}

function formatConfidence(value?: number): string {
  if (typeof value !== 'number') return ''
  return `${Math.round(value * 100)}%`
}

function getSyllabusDetail(row: Question): {
  label: string
  reason?: string
  confidence?: number
  score?: number
} {
  const meta = (row.extraction_metadata as any)?.auto_processing?.syllabus_checking || {}
  const evidence = Array.isArray(meta.evidence) ? meta.evidence[0] : undefined

  return {
    label: syllabusDisplayLabel(row),
    reason: typeof meta.reason === 'string' ? meta.reason : undefined,
    confidence: typeof meta.confidence === 'number' ? meta.confidence : undefined,
    score: typeof meta.score === 'number'
      ? meta.score
      : typeof evidence?.score === 'number'
        ? evidence.score
        : undefined,
  }
}

function syllabusDisplayLabel(row: Question): string {
  if (row.syllabus_checking_status === 'failed') return '失败'
  if (row.syllabus_checking_status === 'paused') return '暂停'
  if (row.syllabus_checking_status === 'pending') return '待筛选'
  if (row.syllabus_scope_result === 'in_scope') return '符合'
  if (row.syllabus_scope_result === 'out_of_scope') return '超纲'
  if (row.syllabus_scope_result === 'uncertain') return '不确定'
  return '—'
}

function syllabusPauseReason(row: Question): string {
  const reason = (row.extraction_metadata as any)?.auto_processing?.syllabus_checking?.reason
  if (row.syllabus_checking_status === 'paused' && typeof reason === 'string') {
    if (reason.includes('未配置考纲') || reason.includes('未关联')) {
      return '未配置考纲'
    }
  }
  if (row.syllabus_checking_status === 'paused') {
    return '考纲已配置，当前题目尚未重新筛选'
  }
  return reason || '暂停'
}

function syllabusTagTheme(row: Question): 'success' | 'warning' | 'danger' | 'default' {
  if (row.syllabus_checking_status === 'failed') return 'danger'
  if (row.syllabus_checking_status === 'paused') return 'warning'
  if (row.syllabus_scope_result === 'in_scope') return 'success'
  if (row.syllabus_scope_result === 'out_of_scope') return 'warning'
  return 'default'
}

// ── Adaptive popup placement ──
const semanticPopupPlacement = ref<'top-left' | 'bottom-left'>('top-left')

function updateSemanticPopupPlacement(e: MouseEvent, estimatedHeight = 240) {
  const el = e.currentTarget as HTMLElement | null
  if (!el) return
  const rect = el.getBoundingClientRect()
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight
  const margin = 16
  const spaceAbove = rect.top
  const spaceBelow = viewportHeight - rect.bottom

  if (spaceAbove < estimatedHeight + margin && spaceBelow > spaceAbove) {
    semanticPopupPlacement.value = 'bottom-left'
    return
  }
  semanticPopupPlacement.value = 'top-left'
}

// ── Syllabus unified filter ──
const syllabusFilterValue = ref('')

function onSyllabusFilterChange() {
  const v = syllabusFilterValue.value
  if (v.startsWith('scope:')) {
    filter.value.syllabus_scope_result = v.slice(6)
    filter.value.syllabus_checking_status = undefined
  } else if (v.startsWith('status:')) {
    filter.value.syllabus_scope_result = undefined
    filter.value.syllabus_checking_status = v.slice(7)
  } else {
    filter.value.syllabus_scope_result = undefined
    filter.value.syllabus_checking_status = undefined
  }
  reloadFromFirstPage()
}

// Guard: if any import dialog opens, close the popup menu
watch(fileImportVisible, (fileVisible) => {
  if (fileVisible) {
    headerImportMenuVisible.value = false
  }
})

onMounted(async () => {
  if (!props.setName) {
    try {
      const set = await getQuestionSet(props.knowledgeBaseId, props.setId)
      fetchedSetName.value = set.name
    } catch { /* ignore */ }
  }
  await loadQuestions()
  startProcessingPolling()
})

onBeforeUnmount(() => {
  stopProcessingPolling()
})

import QuestionEditDialog from './QuestionEditDialog.vue'
import QuestionFileImportDialog from './QuestionFileImportDialog.vue'
import QuestionImportWorkbench from '../QuestionImportWorkbench.vue'
</script>

<style scoped>
.question-set-detail { min-width: 0; }
.detail-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.detail-header h2 { flex: 1; margin: 0; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.filter-bar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.batch-actions { display: flex; align-items: center; gap: 8px; padding: 6px 12px; margin-bottom: 8px; background: var(--td-bg-color-secondarycontainer); border-radius: 6px; }
.batch-label { font-size: 13px; color: var(--td-text-color-secondary); margin-right: 8px; }
.draft-review-tag { cursor: pointer; }
.draft-review-tag:hover { color: var(--td-brand-color); }
.qp-na { color: var(--td-text-color-placeholder); font-size: 12px; }

/* Question table cell styles */
.question-stem-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

.question-match-tag {
  max-width: 150px;
}

.question-match-tag-text {
  display: inline-block;
  max-width: 132px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

/* Semantic popover (knowledge point & syllabus detail) */
.semantic-popover {
  width: 320px;
  max-width: 360px;
  padding: 12px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
  border-radius: 8px;
  box-shadow: var(--td-shadow-2);
  line-height: 1.5;
}

.semantic-popover-title {
  margin-bottom: 10px;
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.semantic-candidate + .semantic-candidate {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--td-component-border);
}

.semantic-candidate-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.semantic-candidate-name {
  min-width: 0;
  flex: 1 1 auto;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.semantic-meta-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 4px;
  font-size: 12px;
}

.semantic-meta-label {
  flex: 0 0 48px;
  color: var(--td-text-color-secondary);
}

.semantic-meta-value {
  min-width: 0;
  flex: 1;
  color: var(--td-text-color-primary);
  word-break: break-word;
}

/* Stem popover */
.semantic-popover-stem {
  width: 360px;
  max-width: min(420px, calc(100vw - 32px));
}

.semantic-stem-text {
  max-height: 180px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.65;
  color: var(--td-text-color-primary);
  white-space: pre-wrap;
  word-break: break-word;
}
.question-empty { padding: 48px 16px; }
.restore-draft-copy { margin: 0; color: var(--td-text-color-secondary); line-height: 1.7; }

/* ── Waterfall timeline styles (aligned with knowledge-processing-timeline) ── */
.kp-timeline {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif;
  font-size: 13px;
  color: var(--td-text-color-primary);
  width: 100%;
  height: 100%;
  overflow: hidden;
}
.kp-shell {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  min-height: 0;
  min-width: 0;
  background: var(--td-bg-color-container);
  overflow: hidden;
}
.kp-head {
  flex: 0 0 auto;
  padding: 14px 20px 10px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}
.kp-head-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.kp-head-doc-title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.35;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.kp-head-status-tag { flex-shrink: 0; }
.kp-head-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  margin-left: auto;
}
.kp-head-meta {
  margin: 8px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  word-break: break-word;
}
.kp-head-meta-sep { margin: 0 6px; color: var(--td-text-color-placeholder); }
.kp-head-meta-part { display: inline; }
.kp-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  background: transparent;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  border-radius: 3px;
  transition: background 150ms ease, color 150ms ease;
}
.kp-icon-btn:hover {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-primary);
}
.kp-body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.kp-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overflow-x: auto;
  padding-bottom: 16px;
}
.kp-ruler {
  flex: 0 0 auto;
  display: grid;
  grid-template-columns: minmax(180px, 40%) 80px 64px 1fr;
  height: 24px;
  align-items: end;
  padding: 12px 20px 6px;
  background: var(--td-bg-color-container);
  border-bottom: 1px dashed var(--td-component-stroke);
  box-shadow: 0 4px 8px -6px rgba(0, 0, 0, 0.12);
}
.kp-ruler-spacer-name, .kp-ruler-spacer-meta { height: 100%; }
.kp-ruler-track { position: relative; height: 100%; margin-right: 16px; }
.kp-tick {
  position: absolute;
  bottom: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  transform: translateX(-50%);
  font-size: 10px;
  color: var(--td-text-color-placeholder);
}
.kp-tick-first { transform: translateX(0); align-items: flex-start; }
.kp-tick-last { transform: translateX(-100%); align-items: flex-end; }
.kp-tick-line { width: 1px; height: 5px; background: var(--td-component-border); }
.kp-tick-label { margin-top: 2px; font-size: 10px; letter-spacing: 0.02em; }
.kp-mono { font-family: "SF Mono", "Fira Code", "Fira Mono", "Roboto Mono", "Menlo", "Courier New", monospace; }
.kp-rows { display: flex; flex-direction: column; }
.kp-row {
  display: grid;
  grid-template-columns: minmax(180px, 40%) 80px 64px 1fr;
  align-items: center;
  height: 32px;
  cursor: default;
  position: relative;
  padding: 0 20px;
  transition: background 150ms ease;
}
.kp-row::before {
  content: "";
  position: absolute;
  left: 0;
  top: 4px;
  bottom: 4px;
  width: 2px;
  background: transparent;
  border-radius: 0 2px 2px 0;
  transition: background 150ms ease;
}
.kp-row:hover { background: var(--td-bg-color-secondarycontainer); }
.kp-row-root { font-weight: 600; }
.kp-row-stage:not(:hover) {
  background: color-mix(in srgb, var(--td-bg-color-secondarycontainer) 55%, transparent);
}
.kp-cell-name { min-width: 0; }
.kp-name-inner { display: flex; align-items: center; gap: 7px; min-width: 0; }
.kp-tree-toggle-spacer { width: 22px; height: 22px; display: inline-block; flex-shrink: 0; margin: -3px 0; }
.kp-status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--td-text-color-placeholder);
}
.kp-dot-done,
.kp-dot-completed { background: var(--td-success-color); }
.kp-dot-failed { background: var(--td-error-color); }
.kp-dot-running { background: var(--td-warning-color); animation: kpLivePulse 1.4s ease-in-out infinite; }
.kp-dot-paused { background: var(--td-warning-color); animation: kpLivePulse 1.4s ease-in-out infinite; }
.kp-dot-pending { background: var(--td-text-color-disabled); }
.kp-name-text {
  font-size: 12px;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.kp-name-root { font-weight: 600; font-size: 13px; }
.kp-name-mono { font-family: "SF Mono", "Fira Code", "Fira Mono", "Roboto Mono", "Menlo", "Courier New", monospace; font-size: 11px; }
.kp-name-kind {
  font-family: "SF Mono", "Fira Code", "Fira Mono", "Roboto Mono", "Menlo", "Courier New", monospace;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--td-text-color-placeholder);
  margin-left: auto;
  padding-left: 8px;
  flex-shrink: 0;
}
.kp-cell-dur {
  font-size: 11px;
  color: var(--td-text-color-secondary);
  text-align: right;
  padding-right: 12px;
  letter-spacing: 0.02em;
}
.kp-running-time { color: var(--td-warning-color); font-weight: 600; }
.kp-cell-bar { position: relative; height: 32px; margin-right: 16px; }
.kp-bar {
  position: absolute;
  top: 12px;
  height: 8px;
  border-radius: 2px;
  background: var(--td-text-color-placeholder);
  min-width: 2px;
  transition: left 800ms cubic-bezier(0.2, 0.8, 0.2, 1), width 800ms cubic-bezier(0.2, 0.8, 0.2, 1);
  z-index: 2;
}
.kp-bar:hover { filter: brightness(1.05); }
.kp-bar-placeholder {
  background: transparent;
  border: 1px dashed var(--td-component-border);
  height: 6px;
  top: 13px;
}
.kp-bar-done,
.kp-bar-completed { background: var(--td-success-color); }
.kp-bar-failed { background: var(--td-error-color); }
.kp-bar-running {
  background-color: var(--td-warning-color-3);
  background-image: linear-gradient(135deg,
    rgba(255,255,255,0.22) 25%, transparent 25%,
    transparent 50%, rgba(255,255,255,0.22) 50%,
    rgba(255,255,255,0.22) 75%, transparent 75%, transparent);
  background-size: 14px 14px;
  animation: kpStripes 1.6s linear infinite;
}
.kp-bar-paused {
  background-color: var(--td-warning-color-3);
  background-image: linear-gradient(135deg,
    rgba(255,255,255,0.22) 25%, transparent 25%,
    transparent 50%, rgba(255,255,255,0.22) 50%,
    rgba(255,255,255,0.22) 75%, transparent 75%, transparent);
  background-size: 14px 14px;
  animation: kpStripes 1.6s linear infinite;
}
.kp-bar-pending { display: none; }
.kp-bar-tip {
  position: absolute;
  bottom: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
  background: var(--td-text-color-primary);
  color: var(--td-text-color-anti);
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 3px;
  white-space: nowrap;
  opacity: 0;
  pointer-events: none;
  transition: opacity 150ms ease;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 4px;
}
.kp-bar-tip-name { font-weight: 500; }
.kp-bar-tip-sep { color: rgba(255,255,255,0.4); }
.kp-bar:hover .kp-bar-tip { opacity: 1; }
.kp-row-root .kp-bar-tip { bottom: auto; top: calc(100% + 8px); }
@keyframes kpStripes { to { background-position: 14px 0; } }
@keyframes kpLivePulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.45; transform: scale(0.8); }
}

/* Status column */
.kp-cell-status {
  display: flex;
  align-items: center;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
.qp-row-status { white-space: nowrap; }
.qp-status-completed,
.qp-status-done { color: var(--td-success-color); }
.qp-status-running { color: var(--td-warning-color); }
.qp-status-paused { color: var(--td-warning-color); }
.qp-status-failed { color: var(--td-error-color); }
.qp-status-pending { color: var(--td-text-color-placeholder); }

/* Detail panel */
.kp-body-with-detail .kp-scroll {
  max-height: 55vh;
}
.kp-detail {
  flex: 0 0 auto;
  max-height: 0;
  overflow: hidden;
  border-top: 1px solid transparent;
  transition: max-height 0.2s ease;
}
.kp-detail-open {
  max-height: 45vh;
  border-top-color: var(--td-component-stroke);
  overflow-y: auto;
}
.kp-detail-head {
  padding: 12px 20px;
  border-bottom: 1px solid var(--td-component-stroke);
}
.kp-detail-title {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}
.kp-detail-dot {
  width: 9px; height: 9px;
}
.kp-detail-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.kp-detail-kind {
  font-family: "SF Mono", "Fira Code", "Fira Mono", "Roboto Mono", "Menlo", "Courier New", monospace;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--td-text-color-placeholder);
  padding-left: 8px;
}
.kp-detail-actions {
  margin-left: auto;
}
.kp-tabs {
  display: flex;
  gap: 0;
  border-bottom: 1px solid var(--td-component-stroke);
  padding: 0 16px;
}
.kp-tab {
  padding: 10px 14px;
  border: none;
  background: transparent;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.15s, border-color 0.15s;
}
.kp-tab:hover { color: var(--td-text-color-primary); }
.kp-tab-active {
  color: var(--td-brand-color);
  border-bottom-color: var(--td-brand-color);
}
.kp-detail-body {
  padding: 12px 20px;
  font-size: 13px;
}
.kp-section { margin-bottom: 16px; }
.kp-section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-placeholder);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin-bottom: 8px;
}
.kp-kv { display: flex; flex-direction: column; gap: 6px; }
.kp-kv-row { display: flex; align-items: baseline; gap: 8px; }
.kp-kv-key {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  min-width: 60px;
  flex-shrink: 0;
}
.kp-kv-val { font-size: 13px; color: var(--td-text-color-primary); }
.kp-status-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 500;
}
.kp-chip-completed,
.kp-chip-done { background: var(--td-success-color-1); color: var(--td-success-color); }
.kp-chip-running { background: var(--td-warning-color-1); color: var(--td-warning-color); }
.kp-chip-paused { background: var(--td-warning-color-1); color: var(--td-warning-color); }
.kp-chip-failed { background: var(--td-error-color-1); color: var(--td-error-color); }
.kp-chip-pending { background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-placeholder); }
.kp-json {
  margin: 0;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 30vh;
  overflow: auto;
  background: var(--td-bg-color-secondarycontainer);
  padding: 10px 12px;
  border-radius: 4px;
}

/* Orange loading for paused button */
.qp-loading-warning :deep(svg),
.qp-loading-warning :deep(circle) {
  color: var(--td-warning-color);
}
.import-type-menu { width: 320px; padding: 6px; }
.import-type-item { width: 100%; display: flex; flex-direction: column; align-items: flex-start; gap: 3px; padding: 10px 12px; border: 0; border-radius: 6px; color: var(--td-text-color-primary); background: transparent; text-align: left; cursor: pointer; }
.import-type-item:not(:disabled):hover { background: var(--td-bg-color-container-hover); }
.import-type-item:disabled { color: var(--td-text-color-disabled); cursor: not-allowed; }
.import-type-title { display: flex; align-items: center; gap: 8px; font-weight: 500; }
.import-type-description,
.import-type-help { color: var(--td-text-color-secondary); font-size: 12px; line-height: 1.5; }
.import-type-item:disabled .import-type-description,
.import-type-item:disabled .import-type-help { color: var(--td-text-color-disabled); }

/* Reprocess menu in waterfall drawer */
.reprocess-menu { width: 180px; padding: 6px; }
.reprocess-item { width: 100%; display: flex; align-items: center; padding: 8px 12px; border: 0; border-radius: 6px; color: var(--td-text-color-primary); background: transparent; text-align: left; cursor: pointer; font-size: 13px; }
.reprocess-item:hover { background: var(--td-bg-color-container-hover); }
</style>

<style>
.import-loading-overlay {
  position: fixed; inset: 0; z-index: 6000;
  display: flex; align-items: center; justify-content: center;
  background: rgba(255,255,255,0.72); backdrop-filter: blur(2px);
  opacity: 1; pointer-events: auto;
  transition: opacity 0.5s ease;
}
.import-loading-overlay.leaving { opacity: 0; pointer-events: none; }
.import-loading-content { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.import-loading-text { font-size: 14px; color: var(--td-text-color-secondary); }

/* Secondary waterfall drawer */
.t-drawer.kp-secondary-drawer .t-drawer__body { padding: 0 !important; }
.t-drawer.kp-secondary-drawer .t-drawer__content { background: var(--td-bg-color-container); }
.t-drawer.kp-secondary-drawer--resizing .t-drawer__content { transition: none !important; }

</style>
