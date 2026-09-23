import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  evidenceInquiryApi,
  type EvidenceInquiryAnswerInput,
  type EvidenceInquiryCreateInput,
  type EvidenceInquiryQuery,
} from '@/api/evidence-inquiry'
import type { EvidenceInquiry } from '@/types/evidence-inquiry'

export const useEvidenceInquiryStore = defineStore('evidence-inquiries', () => {
  const items = ref<EvidenceInquiry[]>([])
  const loading = ref(false)

  const byProposal = computed(() => {
    const grouped = new Map<number, EvidenceInquiry[]>()
    for (const item of items.value) {
      grouped.set(item.proposal_id, [...(grouped.get(item.proposal_id) ?? []), item])
    }
    return grouped
  })

  async function fetch(params?: EvidenceInquiryQuery) {
    loading.value = true
    try {
      const { data } = await evidenceInquiryApi.list(params)
      items.value = data.data
    } finally {
      loading.value = false
    }
  }

  async function refreshProposal(proposalID: number) {
    const { data } = await evidenceInquiryApi.list({ proposal_id: proposalID, page_size: 100 })
    replaceAllForProposal(proposalID, data.data)
    return data.data
  }

  async function create(proposalID: number, body: EvidenceInquiryCreateInput, idempotencyKey: string) {
    try {
      const { data } = await evidenceInquiryApi.create(proposalID, body, idempotencyKey)
      return data.data
    } finally {
      await refreshProposal(proposalID)
    }
  }

  async function answer(id: number, body: EvidenceInquiryAnswerInput) {
    const { data } = await evidenceInquiryApi.answer(id, body)
    const index = items.value.findIndex((item) => item.id === id)
    if (index >= 0) items.value[index] = data.data
    return data.data
  }

  function replaceAllForProposal(proposalID: number, next: EvidenceInquiry[]) {
    items.value = [...items.value.filter((item) => item.proposal_id !== proposalID), ...next]
  }

  return { items, loading, byProposal, fetch, refreshProposal, create, answer }
})
