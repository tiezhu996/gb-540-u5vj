<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ChevronDown, HelpCircle, Plus, RefreshCw } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import ProposalStateBadge from '@/components/common/ProposalStateBadge.vue'
import GeometryEvidenceDrawer from '@/components/common/GeometryEvidenceDrawer.vue'
import TopologyLegend from '@/components/common/TopologyLegend.vue'
import { useBoundaryProposalStore } from '@/stores/boundary-proposal'
import { useLandParcelStore } from '@/stores/land-parcel'
import { useSurveyObservationStore } from '@/stores/survey-observation'
import { useEvidenceInquiryStore } from '@/stores/evidence-inquiry'
import { useAuth } from '@/hooks/useAuth'
import { proposalStateLabel, type ProposalState } from '@/types/enums/proposal-state'
import type { BoundaryProposal } from '@/types/boundary-proposal'
import type { EvidenceInquiry } from '@/types/evidence-inquiry'

const proposals = useBoundaryProposalStore()
const parcels = useLandParcelStore()
const observations = useSurveyObservationStore()
const inquiries = useEvidenceInquiryStore()
const auth = useAuth()
const user = auth.user
const createOpen = ref(false)
const evidenceOpen = ref(false)
const selected = ref<BoundaryProposal | null>(null)
const form = reactive({
  parcel_id: 0,
  base_version: 1,
  proposed_geojson: '{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]}',
  observation_ids: [] as number[],
  snap_tolerance_m: 0.5,
  rationale: '',
})
const inquiryDialog = reactive({
  open: false,
  loading: false,
  saving: false,
  question: '',
  observation_ids: [] as number[],
  idempotencyKey: '',
  responses: {} as Record<number, string>,
})
const inquiryProposal = ref<BoundaryProposal | null>(null)

const transitionTargets: Partial<Record<ProposalState, ProposalState[]>> = {
  draft: ['validated'],
  validated: ['submitted'],
  submitted: ['reviewed'],
  reviewed: ['accepted', 'rejected', 'revision'],
  revision: ['draft'],
}

async function load() {
  await Promise.all([
    parcels.fetch({ page_size: 100 }),
    observations.fetch({ page_size: 100 }),
    proposals.fetch({ page_size: 100 }),
    inquiries.fetch({ page_size: 100 }),
  ])
}

async function create() {
  await proposals.create({ ...form, observation_ids: [...form.observation_ids] })
  createOpen.value = false
  await load()
}

function selectParcel(parcelID: number) {
  form.base_version = parcels.items.find((item) => item.id === parcelID)?.boundary_version ?? 1
  form.observation_ids = []
}

function canTransition(item: BoundaryProposal, to: ProposalState) {
  const actorID = user.value?.id
  if (!actorID) return false
  const creatorStep = to === 'validated' || to === 'submitted' || (to === 'draft' && item.proposal_state === 'revision')
  if (creatorStep) {
    return auth.hasRole('admin') || (auth.hasRole('surveyor', 'gis_analyst') && item.created_by === actorID)
  }
  return auth.hasRole('admin') || (auth.hasRole('reviewer') && item.created_by !== actorID)
}

function availableTransitions(item: BoundaryProposal) {
  return (transitionTargets[item.proposal_state] ?? [])
    .filter((state) => state !== 'accepted' || !pendingInquiry(item))
    .filter((state) => canTransition(item, state))
}

function proposalObservationIDs(item: BoundaryProposal) {
  const raw = item.observation_ids
  const ids = Array.isArray(raw) ? raw : []
  if (ids.length) return ids
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

function observationCount(item: BoundaryProposal) {
  return proposalObservationIDs(item).length
}

async function advance(item: BoundaryProposal, to: ProposalState) {
  await proposals.transition(item.id, { to, version: item.version })
  await inquiries.refreshProposal(item.id)
}

function showEvidence(item: BoundaryProposal) {
  selected.value = item
  evidenceOpen.value = true
}

function proposalInquiries(item: BoundaryProposal | null) {
  return item ? inquiries.byProposal.get(item.id) ?? [] : []
}

function pendingInquiry(item: BoundaryProposal | null) {
  return proposalInquiries(item).find((entry) => entry.status === 'pending')
}

function inquiryCount(item: BoundaryProposal | null) {
  return proposalInquiries(item).length
}

function isReviewer() {
  return auth.hasRole('reviewer', 'admin')
}

function canOpenInquiry(item: BoundaryProposal) {
  const actorID = user.value?.id
  if (!actorID) return false
  if (inquiryCount(item) > 0) return true
  const inReviewWindow = item.proposal_state === 'submitted' || item.proposal_state === 'reviewed'
  if (!inReviewWindow) return false
  return (isReviewer() && item.created_by !== actorID) || item.created_by === actorID
}

function canCreateInquiry(item: BoundaryProposal | null) {
  const actorID = user.value?.id
  return Boolean(
    item &&
      actorID &&
      isReviewer() &&
      item.created_by !== actorID &&
      (item.proposal_state === 'submitted' || item.proposal_state === 'reviewed') &&
      !pendingInquiry(item),
  )
}

async function openInquiries(item: BoundaryProposal) {
  inquiryProposal.value = item
  inquiryDialog.open = true
  inquiryDialog.loading = true
  inquiryDialog.question = ''
  inquiryDialog.observation_ids = []
  inquiryDialog.idempotencyKey = crypto.randomUUID()
  try {
    const history = await inquiries.refreshProposal(item.id)
    inquiryDialog.responses = Object.fromEntries(
      history
        .filter((entry) => entry.status === 'pending' && entry.response === null)
        .map((entry) => [entry.id, '']),
    )
  } finally {
    inquiryDialog.loading = false
  }
}

const inquiryObservationOptions = computed(() => {
  if (!inquiryProposal.value) return []
  const ids = new Set(proposalObservationIDs(inquiryProposal.value))
  return observations.items.filter((item) => ids.has(item.id))
})

const currentInquiries = computed(() => (inquiryProposal.value ? proposalInquiries(inquiryProposal.value) : []))
const currentPendingInquiry = computed(() => (inquiryProposal.value ? pendingInquiry(inquiryProposal.value) : undefined))

const canSubmitInquiry = computed(() =>
  inquiryDialog.question.trim().length >= 4 && inquiryDialog.observation_ids.length > 0,
)

async function submitInquiry() {
  if (!inquiryProposal.value || !canSubmitInquiry.value || inquiryDialog.saving) return
  inquiryDialog.saving = true
  try {
    await inquiries.create(
      inquiryProposal.value.id,
      {
        question: inquiryDialog.question.trim(),
        observation_ids: [...new Set(inquiryDialog.observation_ids)],
      },
      inquiryDialog.idempotencyKey,
    )
    inquiryDialog.question = ''
    inquiryDialog.observation_ids = []
    inquiryDialog.idempotencyKey = crypto.randomUUID()
  } finally {
    inquiryDialog.saving = false
  }
}

async function submitAnswer(entry: EvidenceInquiry) {
  const response = (inquiryDialog.responses[entry.id] ?? '').trim()
  if (response.length < 4) return
  await inquiries.answer(entry.id, { response, version: entry.version })
}

function isProposalAuthor(item: BoundaryProposal | null) {
  return Boolean(item && user.value?.id === item.created_by)
}

onMounted(load)
</script>

<template>
  <PageHeader title="边界提案" eyebrow="BOUNDARY PROPOSALS" description="以地块版本为基线，记录观测证据、吸附容差和内部复核状态。">
    <el-button v-if="auth.hasRole('surveyor', 'gis_analyst', 'admin')" type="primary" @click="createOpen = true"><Plus :size="15" />新建提案</el-button>
  </PageHeader>

  <section class="content-band">
    <div class="toolbar"><el-button @click="load"><RefreshCw :size="15" />刷新</el-button><TopologyLegend /><span class="toolbar-spacer subtle-count">{{ proposals.items.length }} 个提案</span></div>
    <div class="data-surface">
      <el-table v-loading="proposals.loading || inquiries.loading" :data="proposals.items" row-key="id">
        <el-table-column label="提案" width="90"><template #default="scope"><strong>#{{ scope.row.id }}</strong><small class="muted">v{{ scope.row.version }}</small></template></el-table-column>
        <el-table-column label="地块" width="130"><template #default="scope">#{{ scope.row.parcel_id }} · 基线 v{{ scope.row.base_version }}</template></el-table-column>
        <el-table-column label="证据" width="90"><template #default="scope">{{ observationCount(scope.row) }} 条</template></el-table-column>
        <el-table-column label="质询" width="110">
          <template #default="scope">
            <el-button text :disabled="!canOpenInquiry(scope.row)" @click="openInquiries(scope.row)">
              <HelpCircle :size="14" />{{ inquiryCount(scope.row) }} 条<template v-if="pendingInquiry(scope.row)"> · {{ pendingInquiry(scope.row)?.inquiry_number }}</template>
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120"><template #default="scope"><ProposalStateBadge :state="scope.row.proposal_state" /></template></el-table-column>
        <el-table-column label="面积变化" width="125"><template #default="scope"><span :class="scope.row.area_delta_square_m >= 0 ? 'positive' : 'negative'">{{ scope.row.area_delta_square_m >= 0 ? '+' : '' }}{{ scope.row.area_delta_square_m.toFixed(2) }} m²</span></template></el-table-column>
        <el-table-column prop="rationale" label="理由" min-width="180" show-overflow-tooltip />
        <el-table-column label="动作" width="245">
          <template #default="scope">
            <el-button text @click="showEvidence(scope.row)">几何</el-button>
            <el-tooltip v-if="pendingInquiry(scope.row) && canTransition(scope.row, 'accepted')" :content="`接受入口关闭：${pendingInquiry(scope.row)?.inquiry_number} 仍有待回应问题`" placement="top">
              <span><el-button disabled type="warning">接受 · {{ pendingInquiry(scope.row)?.inquiry_number }}</el-button></span>
            </el-tooltip>
            <el-dropdown v-if="availableTransitions(scope.row).length" trigger="click" @command="advance(scope.row, $event)">
              <el-button text type="primary">流转<ChevronDown :size="14" /></el-button>
              <template #dropdown><el-dropdown-menu><el-dropdown-item v-for="target in availableTransitions(scope.row)" :key="target" :command="target">{{ proposalStateLabel[target] }}</el-dropdown-item></el-dropdown-menu></template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!proposals.loading && !proposals.items.length" class="empty-state"><div><strong>暂无提案</strong><span>创建提案后，可在冲突消解页运行确定性检测。</span></div></div>
    </div>
  </section>

  <el-dialog v-model="createOpen" title="新建边界提案" width="min(680px, calc(100vw - 28px))">
    <el-form label-position="top">
      <div class="form-grid">
        <el-form-item label="地块"><el-select v-model="form.parcel_id" placeholder="选择地块" style="width: 100%" @change="selectParcel"><el-option v-for="parcel in parcels.items" :key="parcel.id" :label="`${parcel.parcel_code} · v${parcel.boundary_version}`" :value="parcel.id" /></el-select></el-form-item>
        <el-form-item label="基线版本"><el-input-number v-model="form.base_version" :min="1" style="width: 100%" /></el-form-item>
        <el-form-item label="吸附容差（m）"><el-input-number v-model="form.snap_tolerance_m" :min="0.01" :max="1000" :precision="2" style="width: 100%" /></el-form-item>
        <el-form-item label="观测证据"><el-select v-model="form.observation_ids" multiple collapse-tags collapse-tags-tooltip placeholder="选择观测" style="width: 100%"><el-option v-for="observation in observations.items.filter((item) => item.parcel_id === form.parcel_id)" :key="observation.id" :label="`${observation.observation_code} · ±${observation.horizontal_accuracy_m}m`" :value="observation.id" /></el-select></el-form-item>
      </div>
      <el-form-item label="提案边界 GeoJSON"><el-input v-model="form.proposed_geojson" type="textarea" :rows="5" /></el-form-item>
      <el-form-item label="理由"><el-input v-model="form.rationale" type="textarea" :rows="3" maxlength="2000" show-word-limit /></el-form-item>
    </el-form>
    <template #footer><el-button @click="createOpen = false">取消</el-button><el-button type="primary" :disabled="!form.parcel_id || !form.rationale" @click="create">保存提案</el-button></template>
  </el-dialog>

  <el-dialog v-model="inquiryDialog.open" :title="inquiryProposal ? `证据质询 · 提案 #${inquiryProposal.id}` : '证据质询'" width="min(780px, calc(100vw - 28px))">
    <div v-loading="inquiryDialog.loading" class="inquiry-panel">
      <el-alert v-if="currentPendingInquiry" type="warning" :closable="false" :title="`${currentPendingInquiry.inquiry_number} 仍有待回应问题，接受入口保持关闭`" />
      <el-alert v-else-if="inquiryProposal && isReviewer() && inquiryProposal.created_by !== user?.id && !canCreateInquiry(inquiryProposal)" type="info" :closable="false" title="仅在提案提交或复核阶段发起新质询；历史质询仍可复核留档。" />

      <template v-if="canCreateInquiry(inquiryProposal)">
        <el-divider content-position="left">发起新的证据质询</el-divider>
        <el-form label-position="top">
          <el-form-item label="需要作者补充说明的观测">
            <el-select v-model="inquiryDialog.observation_ids" multiple collapse-tags collapse-tags-tooltip placeholder="仅可选择本提案引用的测量观测" style="width: 100%">
              <el-option v-for="observation in inquiryObservationOptions" :key="observation.id" :label="`${observation.observation_code} · ±${observation.horizontal_accuracy_m}m`" :value="observation.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="复核疑问"><el-input v-model="inquiryDialog.question" type="textarea" :rows="3" maxlength="1000" show-word-limit placeholder="说明这些观测为何还不足以支撑改线，以及需要补充的证据。" /></el-form-item>
        </el-form>
        <div class="dialog-actions"><el-button type="primary" :loading="inquiryDialog.saving" :disabled="!canSubmitInquiry" @click="submitInquiry">发出质询</el-button></div>
      </template>

      <el-divider content-position="left">历史质询与原观测留档</el-divider>
      <el-empty v-if="!currentInquiries.length" description="暂无质询记录" :image-size="70" />
      <article v-for="entry in currentInquiries" :key="entry.id" class="inquiry-card">
        <header>
          <div><strong>{{ entry.inquiry_number }}</strong><el-tag size="small" :type="entry.status === 'pending' ? 'warning' : 'success'">{{ entry.status === 'pending' ? '待回应' : '已回应' }}</el-tag></div>
          <span class="muted">{{ entry.created_by_name }} · {{ entry.created_at }}</span>
        </header>
        <p class="question">{{ entry.question }}</p>
        <div class="observation-list">
          <span v-for="observation in entry.observations" :key="observation.id" class="observation-chip" :title="observation.point_geojson">
            {{ observation.observation_code }} · {{ observation.method }} · ±{{ observation.horizontal_accuracy_m }}m
          </span>
        </div>
        <template v-if="entry.response">
          <el-divider />
          <p class="answer"><strong>{{ entry.responded_by_name || '作者' }}补充：</strong>{{ entry.response }}</p>
          <small class="muted">{{ entry.responded_at }}</small>
        </template>
        <template v-else-if="isProposalAuthor(inquiryProposal)">
          <el-divider />
          <el-input v-model="inquiryDialog.responses[entry.id]" type="textarea" :rows="3" maxlength="2000" show-word-limit placeholder="请针对指定观测补充测量依据、精度说明或与改线的关系。" />
          <div class="dialog-actions"><el-button type="primary" :disabled="(inquiryDialog.responses[entry.id] ?? '').trim().length < 4" @click="submitAnswer(entry)">提交补充说明</el-button></div>
        </template>
      </article>
    </div>
  </el-dialog>

  <GeometryEvidenceDrawer v-model="evidenceOpen" title="提案几何" :geometry="selected?.proposed_geojson" :explanation="selected ? `提案 #${selected.id} · 吸附容差 ${selected.snap_tolerance_m} m` : ''" />
</template>

<style scoped>
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.muted { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.positive { color: #17604e; }.negative { color: #9c3028; }
.inquiry-panel { display: grid; gap: 12px; max-height: 70vh; overflow-y: auto; padding-right: 4px; }
.inquiry-card { padding: 14px; border: 1px solid var(--line); border-radius: 5px; background: var(--surface); }
.inquiry-card header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.inquiry-card header div { display: flex; align-items: center; gap: 8px; }
.question, .answer { margin: 10px 0 0; line-height: 1.65; }
.observation-list { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.observation-chip { display: inline-flex; padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; color: #2a5d78; background: #e8f2f7; font-size: 12px; }
.dialog-actions { display: flex; justify-content: flex-end; margin-top: 10px; }
@media (max-width: 620px) { .form-grid { grid-template-columns: 1fr; } .inquiry-card header { align-items: flex-start; flex-direction: column; } }
</style>
