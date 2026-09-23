import type { ChallengeState } from './enums/challenge-state'

export interface EvidenceChallenge {
  id: number
  proposal_id: number
  sequence: number
  challenge_code: string
  observation_ids: number[] | string
  question: string
  response: string
  challenge_state: ChallengeState
  raised_by: number
  responded_by?: number | null
  responded_at?: string | null
  created_at: string
  updated_at: string
}
