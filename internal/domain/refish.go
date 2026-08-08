package domain

// Re-fishing: offering a new slot to something that did not get done.
//
// The distinction that makes this humane rather than nagging is in TimeBlock's
// own comment — an unfinished block whose TIME was never the point (exercise,
// reading, an errand) can be offered another slot; a booking cannot, because a
// lecture you missed is missed and re-offering it is a lie about what happened.
//
// The chain and the cap are the anti-shame half. Without a cap the system keeps
// pushing the same undone thing forward forever, which is precisely the debt
// spiral this product exists not to create: after a few honest tries, "you have
// not done this" stops being information and becomes an accusation.

// RescheduleCap is how many times one thing may be re-offered before the system
// stops asking. Three is a product value: enough that a genuinely busy week
// does not lose the intention, few enough that the fourth silence is read as an
// answer rather than an oversight.
const RescheduleCap = 3

// Refishable reports whether an unfinished block may be offered a new slot.
//
// Completed blocks are done. Appointments are bookings — their time WAS the
// point. Achievements are records of something that happened, not plans. And a
// block that has already used up the cap has been asked about enough.
func (b TimeBlock) Refishable() bool {
	switch {
	case b.Completed, b.IsAchievement:
		return false
	case b.Type == BlockAppointment:
		return false
	case b.RescheduleCount >= RescheduleCap:
		return false
	}
	return true
}

// RescheduleChain is where a retry says it came from, and how many tries deep
// it is.
//
// The count is derived from the ORIGINAL rather than taken from the caller: a
// client that reported its own attempt number could reset it and the cap would
// mean nothing. The root id is carried forward unchanged so a chain three deep
// still points at the thing the user actually wanted, not at the previous
// attempt — otherwise "what happened to that" walks a linked list.
func RescheduleChain(original TimeBlock) (rootID string, count int) {
	rootID = original.RescheduledFrom
	if rootID == "" {
		rootID = original.ID
	}
	return rootID, original.RescheduleCount + 1
}
