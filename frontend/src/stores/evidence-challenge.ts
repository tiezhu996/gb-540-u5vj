import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/api/client'
import type { ApiEnvelope } from '@/types/api'
import { evidenceChallengeApi, type ChallengeCreateInput } from '@/api/evidence-challenge'
import type { EvidenceChallenge } from '@/types/evidence-challenge'

export const useEvidenceChallengeStore = defineStore('evidence-challenges', () => {
  const items = ref<EvidenceChallenge[]>([])
  const loading = ref(false)

  function normalizeIds(raw: number[] | string): number[] {
    if (Array.isArray(raw)) return raw
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  function upsert(item: EvidenceChallenge) {
    const index = items.value.findIndex((current) => current.id === item.id)
    if (index >= 0) items.value[index] = item
    else items.value.push(item)
  }

  function forProposal(proposalId: number): EvidenceChallenge[] {
    return items.value
      .filter((item) => item.proposal_id === proposalId)
      .sort((a, b) => a.sequence - b.sequence)
  }

  function pendingForProposal(proposalId: number): EvidenceChallenge[] {
    return items.value
      .filter((item) => item.proposal_id === proposalId && item.challenge_state === 'pending')
      .sort((a, b) => a.sequence - b.sequence)
  }

  async function fetchForProposal(proposalId: number) {
    loading.value = true
    try {
      const { data } = await evidenceChallengeApi.list(proposalId, { page_size: 100 })
      items.value = items.value.filter((item) => item.proposal_id !== proposalId)
      items.value.push(...data.data)
      return data.data
    } finally {
      loading.value = false
    }
  }

  // Fetch every unanswered challenge once so the acceptance entry stays closed
  // across the whole proposal table without issuing per-row requests.
  async function fetchPending() {
    const { data } = await api.get<ApiEnvelope<EvidenceChallenge[]>>('/evidence-challenges', {
      params: { state: 'pending', page_size: 100 },
    })
    items.value = items.value.filter((item) => item.challenge_state !== 'pending')
    items.value.push(...data.data)
    return data.data
  }

  async function raise(proposalId: number, body: ChallengeCreateInput, idempotencyKey: string) {
    const { data } = await evidenceChallengeApi.raise(proposalId, body, idempotencyKey)
    upsert(data.data)
    return data.data
  }

  async function respond(challengeId: number, response: string) {
    const { data } = await evidenceChallengeApi.respond(challengeId, { response })
    upsert(data.data)
    return data.data
  }

  return { items, loading, normalizeIds, forProposal, pendingForProposal, fetchForProposal, fetchPending, raise, respond }
})
