export interface Participant { id: string; name: string; order: number }
export interface Assignment { mode: 'single' | 'equal' | 'all' | 'units' | 'weighted'; weights: { participant_id: string; value: number }[] }
export interface Item { id: string; name: string; quantity: number; unit_price: number | null; assignment: Assignment | null }
export interface Bill { participants: Participant[]; payer_id: string; receipt_total: number | null; items: Item[] }
export interface Meeting {
  id: string; version: number; state: 'draft' | 'finalized' | 'closed'; currency: string
  title: string; description: string; date: string; venue: string; bill: Bill
  calculation: {
    total: number; unassigned_amount: number; unassigned_ids: string[]; blockers: string[]
    to_repay: number | null
    totals: { participant_id: string; amount: number; debt: number | null }[]
    lines: { item_id: string; amount: number; shares: { participant_id: string; amount: number }[] }[]
  }
  payments: { id: string; participant_id: string; amount: number; cancelled_at: string | null; cancellation_reason: string | null }[]
  remaining: Record<string, number>
}

export interface TransferProfile { name: string; phone: string; bank: string }
export interface Friend { id: string; name: string; phone: string; birthday: string; description: string }
export interface FriendMeeting { id: string; title: string; date: string; venue: string; state: Meeting['state']; amount: number; item_count: number }
export interface FriendStats {
  meeting_count: number; confirmed_count: number; total_spent: number; average_spent: number
  biggest_spent: number; outstanding: number; meetings: FriendMeeting[]
}
export interface FriendDetails { friend: Friend; stats: FriendStats }
