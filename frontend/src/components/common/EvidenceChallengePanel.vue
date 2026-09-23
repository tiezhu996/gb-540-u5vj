<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useEvidenceChallengeStore } from '@/stores/evidence-challenge'
import { useSurveyObservationStore } from '@/stores/survey-observation'
import { useAuth } from '@/hooks/useAuth'
import { challengeStateLabel } from '@/types/enums/challenge-state'
import type { BoundaryProposal } from '@/types/boundary-proposal'
import type { EvidenceChallenge } from '@/types/evidence-challenge'

const props = defineProps<{ modelValue: boolean; proposal: BoundaryProposal | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; changed: [] }>()

const challengeStore = useEvidenceChallengeStore()
const observationStore = useSurveyObservationStore()
const auth = useAuth()

const submitting = ref(false)
const raiseForm = reactive({ observation_ids: [] as number[], question: '' })
const responses = reactive<Record<number, string>>({})
// One stable key per opened form: network retries or a repeated click replay
// the same challenge instead of producing a second pending record.
const raiseKey = ref('')

const challenges = computed(() => (props.proposal ? challengeStore.forProposal(props.proposal.id) : []))
const pending = computed(() => (props.proposal ? challengeStore.pendingForProposal(props.proposal.id) : []))
const citedObservationIds = computed<number[]>(() => (props.proposal ? challengeStore.normalizeIds(props.proposal.observation_ids) : []))
const citedObservations = computed(() => observationStore.items.filter((item) => citedObservationIds.value.includes(item.id)))

const canRaise = computed(() => {
  if (!props.proposal) return false
  const actorID = auth.user.value?.id
  return Boolean(
    actorID &&
      (auth.hasRole('reviewer', 'admin') && props.proposal.created_by !== actorID) &&
      (props.proposal.proposal_state === 'submitted' || props.proposal.proposal_state === 'reviewed'),
  )
})

const canAnswer = computed(() => {
  if (!props.proposal) return false
  const actorID = auth.user.value?.id
  const decided = props.proposal.proposal_state === 'accepted' || props.proposal.proposal_state === 'rejected'
  return Boolean(actorID && !decided && (auth.hasRole('admin') || props.proposal.created_by === actorID))
})

function observationLabel(id: number) {
  const found = observationStore.items.find((item) => item.id === id)
  return found ? `${found.observation_code} · ±${found.horizontal_accuracy_m}m` : `观测 #${id}`
}

function citedIds(challenge: EvidenceChallenge) {
  return challengeStore.normalizeIds(challenge.observation_ids)
}

watch(
  () => props.modelValue,
  async (open) => {
    if (open && props.proposal) {
      raiseForm.observation_ids = []
      raiseForm.question = ''
      raiseKey.value = `ec-${props.proposal.id}-${crypto.randomUUID()}`
      await challengeStore.fetchForProposal(props.proposal.id)
    }
  },
)

async function submitChallenge() {
  if (!props.proposal) return
  if (!raiseForm.observation_ids.length) {
    ElMessage.warning('请至少指定一条被质询的观测')
    return
  }
  if (!raiseForm.question.trim()) {
    ElMessage.warning('请写明疑问内容')
    return
  }
  submitting.value = true
  try {
    await challengeStore.raise(props.proposal.id, { observation_ids: [...raiseForm.observation_ids], question: raiseForm.question.trim() }, raiseKey.value)
    raiseForm.observation_ids = []
    raiseForm.question = ''
    raiseKey.value = `ec-${props.proposal.id}-${crypto.randomUUID()}`
    emit('changed')
  } finally {
    submitting.value = false
  }
}

async function submitResponse(challenge: EvidenceChallenge) {
  const answer = (responses[challenge.id] ?? '').trim()
  if (!answer) {
    ElMessage.warning('请先填写补充说明')
    return
  }
  submitting.value = true
  try {
    await challengeStore.respond(challenge.id, answer)
    responses[challenge.id] = ''
    emit('changed')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-drawer :model-value="modelValue" :title="proposal ? `证据质询 · 提案 #${proposal.id}` : '证据质询'" size="min(560px, 94vw)" @update:model-value="emit('update:modelValue', $event)">
    <el-alert v-if="pending.length" type="warning" :closable="false" show-icon>
      <template #title>仍有 {{ pending.length }} 条待回应质询：{{ pending.map((item) => item.challenge_code).join('、') }}；全部回应前接受入口保持关闭。</template>
    </el-alert>
    <el-alert v-else-if="challenges.length" type="success" :closable="false" show-icon title="全部质询均已回应，复核员可以继续接受提案。" />

    <div v-if="canRaise" class="raise-card">
      <h4>发起证据质询</h4>
      <el-form label-position="top">
        <el-form-item label="指定观测">
          <el-select v-model="raiseForm.observation_ids" multiple collapse-tags collapse-tags-tooltip placeholder="选择支撑不足的观测" style="width: 100%">
            <el-option v-for="observation in citedObservations" :key="observation.id" :label="`${observation.observation_code} · ±${observation.horizontal_accuracy_m}m`" :value="observation.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="疑问">
          <el-input v-model="raiseForm.question" type="textarea" :rows="3" maxlength="2000" show-word-limit placeholder="说明哪些测量观测不足以支撑改线，需要作者补充什么证据。" />
        </el-form-item>
      </el-form>
      <div class="raise-actions">
        <el-button type="primary" :loading="submitting" :disabled="!citedObservationIds.length" @click="submitChallenge">发出质询</el-button>
        <span v-if="!citedObservationIds.length" class="hint">该提案没有引用任何观测，无法发起证据质询。</span>
      </div>
    </div>

    <div v-if="!challenges.length" class="challenge-empty">暂无证据质询记录。</div>
    <article v-for="challenge in challenges" :key="challenge.id" :class="['challenge-card', challenge.challenge_state]">
      <header>
        <strong>{{ challenge.challenge_code }}</strong>
        <span :class="['state-tag', challenge.challenge_state]">{{ challengeStateLabel[challenge.challenge_state] }}</span>
      </header>
      <ul class="observation-refs">
        <li v-for="observationId in citedIds(challenge)" :key="observationId">{{ observationLabel(observationId) }}</li>
      </ul>
      <p class="question-text">{{ challenge.question }}</p>
      <small class="meta-line">复核员 #{{ challenge.raised_by }} · {{ new Date(challenge.created_at).toLocaleString() }}</small>
      <div v-if="challenge.response" class="answer-block">
        <strong>作者补充说明</strong>
        <p>{{ challenge.response }}</p>
        <small class="meta-line">回应人 #{{ challenge.responded_by }}<template v-if="challenge.responded_at"> · {{ new Date(challenge.responded_at).toLocaleString() }}</template></small>
      </div>
      <div v-else-if="canAnswer" class="answer-form">
        <el-input v-model="responses[challenge.id]" type="textarea" :rows="3" maxlength="2000" show-word-limit placeholder="针对指定观测补充测量方式、精度或校核证据。" />
        <el-button type="primary" plain size="small" :loading="submitting" @click="submitResponse(challenge)">提交补充说明</el-button>
      </div>
      <small v-else-if="challenge.challenge_state === 'pending'" class="hint">等待提案作者补充说明。</small>
    </article>
  </el-drawer>
</template>

<style scoped>
.raise-card { margin: 14px 0 18px; padding: 14px; border: 1px solid var(--line); background: var(--surface-strong); }
.raise-card h4 { margin: 0 0 10px; font-size: 14px; }
.raise-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.hint { color: var(--text-muted); font-size: 12px; }
.challenge-empty { padding: 26px 0; color: var(--text-muted); font-size: 13px; text-align: center; }
.challenge-card { margin-top: 12px; padding: 14px; border: 1px solid var(--line); border-left-width: 3px; background: var(--surface); }
.challenge-card.pending { border-left-color: var(--warning); }
.challenge-card.answered { border-left-color: var(--accent); }
.challenge-card header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.state-tag { padding: 2px 8px; border: 1px solid var(--line-strong); border-radius: 3px; font-size: 11px; font-weight: 800; color: var(--text-muted); }
.state-tag.pending { color: #755310; border-color: #e0c16b; background: #fff5db; }
.state-tag.answered { color: #17604e; border-color: #9dcebd; background: #e8f4f0; }
.observation-refs { display: flex; flex-wrap: wrap; gap: 6px; margin: 10px 0; padding: 0; list-style: none; }
.observation-refs li { padding: 2px 8px; border: 1px solid var(--line); background: var(--surface-strong); font-size: 12px; }
.question-text { margin: 0 0 8px; color: var(--text); font-size: 13px; line-height: 1.6; white-space: pre-wrap; }
.meta-line { display: block; color: var(--text-muted); font-size: 11px; }
.answer-block { margin-top: 10px; padding: 10px 12px; border: 1px solid #9dcebd; background: #f2f8f5; }
.answer-block strong { font-size: 12px; color: #17604e; }
.answer-block p { margin: 6px 0; font-size: 13px; line-height: 1.6; white-space: pre-wrap; }
.answer-form { display: grid; gap: 8px; margin-top: 10px; }
</style>
