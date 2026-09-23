export const CHALLENGE_STATES = ['pending', 'answered'] as const
export type ChallengeState = (typeof CHALLENGE_STATES)[number]
export const challengeStateLabel: Record<ChallengeState, string> = { pending: '待回应', answered: '已回应' }
