<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ChevronDown, Plus, RefreshCw, MessagesSquare } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import ProposalStateBadge from '@/components/common/ProposalStateBadge.vue'
import GeometryEvidenceDrawer from '@/components/common/GeometryEvidenceDrawer.vue'
import EvidenceChallengePanel from '@/components/common/EvidenceChallengePanel.vue'
import TopologyLegend from '@/components/common/TopologyLegend.vue'
import { useBoundaryProposalStore } from '@/stores/boundary-proposal'
import { useLandParcelStore } from '@/stores/land-parcel'
import { useSurveyObservationStore } from '@/stores/survey-observation'
import { useEvidenceChallengeStore } from '@/stores/evidence-challenge'
import { useAuth } from '@/hooks/useAuth'
import { proposalStateLabel, type ProposalState } from '@/types/enums/proposal-state'
import type { BoundaryProposal } from '@/types/boundary-proposal'

const proposals = useBoundaryProposalStore()
const parcels = useLandParcelStore()
const observations = useSurveyObservationStore()
const challenges = useEvidenceChallengeStore()
const auth = useAuth()
const createOpen = ref(false)
const evidenceOpen = ref(false)
const challengeOpen = ref(false)
const selected = ref<BoundaryProposal | null>(null)
const form = reactive({
  parcel_id: 0,
  base_version: 1,
  proposed_geojson: '{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]}',
  observation_ids: [] as number[],
  snap_tolerance_m: 0.5,
  rationale: '',
})

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
    challenges.fetchPending(),
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
  const actorID = auth.user.value?.id
  if (!actorID) return false
  // Acceptance stays closed while cited evidence questions are unanswered;
  // rejection and revision remain available to the reviewer.
  if (to === 'accepted' && challenges.pendingForProposal(item.id).length) return false
  const creatorStep = to === 'validated' || to === 'submitted' || (to === 'draft' && item.proposal_state === 'revision')
  if (creatorStep) {
    return auth.hasRole('admin') || (auth.hasRole('surveyor', 'gis_analyst') && item.created_by === actorID)
  }
  return auth.hasRole('admin') || (auth.hasRole('reviewer') && item.created_by !== actorID)
}

function transitionDisabledReason(item: BoundaryProposal, to: ProposalState) {
  if (to === 'accepted') {
    const open = challenges.pendingForProposal(item.id)
    if (open.length) return `接受入口关闭：待回应质询 ${open.map((entry) => entry.challenge_code).join('、')}`
  }
  return ''
}

function observationCount(item: BoundaryProposal) {
  if (Array.isArray(item.observation_ids)) return item.observation_ids.length
  try {
    const parsed = JSON.parse(item.observation_ids)
    return Array.isArray(parsed) ? parsed.length : 0
  } catch {
    return item.observation_ids.split(',').filter(Boolean).length
  }
}

async function advance(item: BoundaryProposal, to: ProposalState) {
  await proposals.transition(item.id, { to, version: item.version })
  if (to === 'accepted') await challenges.fetchPending()
}

function pendingChallengeCodes(item: BoundaryProposal) {
  return challenges.pendingForProposal(item.id).map((entry) => entry.challenge_code)
}

function transitionsFor(item: BoundaryProposal): ProposalState[] {
  return transitionTargets[item.proposal_state] ?? []
}

function showEvidence(item: BoundaryProposal) {
  selected.value = item
  evidenceOpen.value = true
}

function showChallenges(item: BoundaryProposal) {
  selected.value = item
  challengeOpen.value = true
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
      <el-table v-loading="proposals.loading" :data="proposals.items" row-key="id">
        <el-table-column label="提案" width="90"><template #default="scope"><strong>#{{ scope.row.id }}</strong><small class="muted">v{{ scope.row.version }}</small></template></el-table-column>
        <el-table-column label="地块" width="130"><template #default="scope">#{{ scope.row.parcel_id }} · 基线 v{{ scope.row.base_version }}</template></el-table-column>
        <el-table-column label="证据" width="150">
          <template #default="scope">
            <span>{{ observationCount(scope.row) }} 条观测</span>
            <el-button v-if="challenges.forProposal(scope.row.id).length" link type="warning" size="small" @click="showChallenges(scope.row)">
              {{ challenges.pendingForProposal(scope.row.id).length }} 待回应 / {{ challenges.forProposal(scope.row.id).length }} 质询
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120"><template #default="scope"><ProposalStateBadge :state="scope.row.proposal_state" /></template></el-table-column>
        <el-table-column label="面积变化" width="125"><template #default="scope"><span :class="scope.row.area_delta_square_m >= 0 ? 'positive' : 'negative'">{{ scope.row.area_delta_square_m >= 0 ? '+' : '' }}{{ scope.row.area_delta_square_m.toFixed(2) }} m²</span></template></el-table-column>
        <el-table-column prop="rationale" label="理由" min-width="200" show-overflow-tooltip />
        <el-table-column label="动作" width="250">
          <template #default="scope">
            <el-button text @click="showEvidence(scope.row)">几何</el-button>
            <el-tooltip v-if="scope.row.proposal_state === 'submitted' || scope.row.proposal_state === 'reviewed'" placement="top" :disabled="!pendingChallengeCodes(scope.row).length" :content="`接受入口关闭：待回应质询 ${pendingChallengeCodes(scope.row).join('、')}`">
              <span><el-button text :type="pendingChallengeCodes(scope.row).length ? 'warning' : 'primary'" @click="showChallenges(scope.row)"><MessagesSquare :size="14" />质询<span v-if="pendingChallengeCodes(scope.row).length" class="challenge-count">{{ pendingChallengeCodes(scope.row).length }}</span></el-button></span>
            </el-tooltip>
            <el-button v-else-if="challenges.forProposal(scope.row.id).length" text type="primary" @click="showChallenges(scope.row)"><MessagesSquare :size="14" />质询</el-button>
            <el-dropdown v-if="transitionsFor(scope.row).length" trigger="click" @command="advance(scope.row, $event)">
              <el-button text type="primary">流转<ChevronDown :size="14" /></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item
                    v-for="target in transitionsFor(scope.row)"
                    :key="target"
                    :command="target"
                    :disabled="!canTransition(scope.row, target)"
                  >{{ proposalStateLabel[target] }}<span v-if="!canTransition(scope.row, target) && transitionDisabledReason(scope.row, target)" class="dropdown-hint">（{{ transitionDisabledReason(scope.row, target).replace('接受入口关闭：', '') }}）</span></el-dropdown-item>
                </el-dropdown-menu>
              </template>
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

  <GeometryEvidenceDrawer v-model="evidenceOpen" title="提案几何" :geometry="selected?.proposed_geojson" :explanation="selected ? `提案 #${selected.id} · 吸附容差 ${selected.snap_tolerance_m} m` : ''" />

  <EvidenceChallengePanel v-model="challengeOpen" :proposal="selected" @changed="challenges.fetchPending()" />
</template>

<style scoped>
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.muted { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.challenge-count { display: inline-grid; place-items: center; min-width: 17px; height: 17px; margin-left: 4px; padding: 0 4px; border-radius: 9px; background: #a66d0b; color: #fff; font-size: 10px; font-weight: 800; }
.dropdown-hint { margin-left: 6px; color: #a66d0b; font-size: 11px; font-weight: 700; }
.positive { color: #17604e; }.negative { color: #9c3028; }
@media (max-width: 620px) { .form-grid { grid-template-columns: 1fr; } }
</style>
