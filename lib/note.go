// Copyright 2019 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.
// Derived from the "FeedSync" sample code which was covered
// by the Microsoft Public License (Ms-Pl) of December 3, 2007.
// FeedSync itself was derived from the algorithms underlying
// Lotus Notes replication, developed by Ray Ozzie et al c.1985.

package notelib

import (
	"github.com/blues/note-go/note"
	"time"
)

// UpdateNote performs the bulk of Note Update, Delete, Merge operations
func UpdateNote(xnote *note.Note, endpointID string, resolveConflicts bool, deleted bool) {
	var when int64
	var where string

	updates := xnote.Updates + 1
	sequence := updates

	// Purge the history of other updates performed by the endpoint doing this update
	newHistories := copyOrCreateBlankHistory(nil)

	if xnote.Histories != nil {
		histories := *xnote.Histories
		for _, History := range histories {

			// Bump sequence if the same endpoint is updating the note
			if History.EndpointID == endpointID && History.Sequence >= sequence {
				sequence = History.Sequence + 1
			}

			//  Sparse purge
			if History.EndpointID != endpointID {
				newHistories = append(newHistories, History)
			}
		}
	}

	xnote.Histories = &newHistories

	// Create a new history
	newHistory := NewHistory(endpointID, when, where, sequence)

	// Insert newHistory at offset 0, then append the old history
	histories := copyOrCreateBlankHistory(nil)
	histories = append(histories, newHistory)
	if xnote.Histories != nil && len(*xnote.Histories) > 0 {
		histories = append(histories, *xnote.Histories...)
	}
	xnote.Histories = &histories

	// Update the note with these key flags
	xnote.Updates = updates
	xnote.Deleted = deleted

	//  Unless performing implicit conflict resolution, this code resolves all conflicts
	//  but does not accomodate selective conflict resolution
	performConflictResolution := resolveConflicts
	performImplicitConflictResolution := !performConflictResolution

	if performConflictResolution || performImplicitConflictResolution {
		noteConflicts := copyOrCreateBlankConflict(xnote.Conflicts)
		noteHistories := copyOrCreateNonblankHistory(xnote.Histories)

		for conflictIndex := len(noteConflicts) - 1; conflictIndex >= 0; conflictIndex-- {
			conflict := noteConflicts[conflictIndex]
			conflictNote := conflict
			conflictNoteHistories := copyOrCreateNonblankHistory(conflictNote.Histories)

			if performImplicitConflictResolution && (conflictNoteHistories[0].EndpointID != endpointID) {
				continue
			}

			for conflictHistoryIndex := 0; conflictHistoryIndex < len(conflictNoteHistories); conflictHistoryIndex++ {
				conflictHistory := conflictNoteHistories[conflictHistoryIndex]

				isSubsumed := false

				for historyIndex := 0; historyIndex < len(noteHistories); historyIndex++ {
					history := noteHistories[historyIndex]

					if isHistorySubsumedBy(conflictHistory, history) {
						isSubsumed = true
						break
					}

					if (history.EndpointID == conflictHistory.EndpointID) && (conflictHistory.Sequence >= sequence) {
						sequence = conflictHistory.Sequence + 1
						noteHistories[historyIndex].Sequence = sequence
					}
				}

				if !isSubsumed {
					//  Attempt to sparse purge again before incorporating conflict history
					for historyIndex := len(noteHistories) - 1; historyIndex >= 0; historyIndex-- {
						history := noteHistories[historyIndex]
						if isHistorySubsumedBy(history, conflictHistory) {
							// Delete the note at historyIndex
							if historyIndex < len(noteHistories) {
								noteHistories = append(noteHistories[:historyIndex], noteHistories[historyIndex+1:]...)
							}
						}
					}

					// Insert conflictHistory at offset 1
					noteHistories = append(noteHistories, conflictHistory)
					if len(noteHistories) > 1 {
						copy(noteHistories[2:], noteHistories[1:])
						noteHistories[1] = conflictHistory
					}
				}
			}

			// Delete the note at conflictIndex
			if conflictIndex < len(noteConflicts) {
				noteConflicts = append(noteConflicts[:conflictIndex], noteConflicts[conflictIndex+1:]...)
			}

		}

		if performConflictResolution {
			noteConflicts = copyOrCreateBlankConflict(nil)
		}

		xnote.Conflicts = &noteConflicts
		xnote.Histories = &noteHistories

	}

}

// CompareModified compares two Notes, and returns
//     1 if local note is newer than incoming note
//    -1 if incoming note is newer than local note
//     0 if notes are equal
//  conflictDataDiffers is returned as true if they
//     are equal but their conflict data is different
func CompareModified(xnote note.Note, incomingNote note.Note) (conflictDataDiffers bool, result int) {

	if incomingNote.Updates > xnote.Updates {
		return false, -1
	}
	if incomingNote.Updates < xnote.Updates {
		return false, 1
	}

	noteHistories := copyOrCreateNonblankHistory(xnote.Histories)
	incomingNoteHistories := copyOrCreateNonblankHistory(incomingNote.Histories)
	noteConflicts := copyOrCreateBlankConflict(xnote.Conflicts)
	incomingNoteConflicts := copyOrCreateBlankConflict(incomingNote.Conflicts)

	localTopMostHistory := noteHistories[0]
	incomingTopMostHistory := incomingNoteHistories[0]

	if localTopMostHistory.When == 0 {
		if incomingTopMostHistory.When != 0 {
			return false, -1
		}
	}

	// Compare when modified
	if localTopMostHistory.When != 0 && incomingTopMostHistory.When != 0 {
		result := compareNoteTimestamps(localTopMostHistory.When, incomingTopMostHistory.When)
		if result != 0 {
			return false, result
		}
	}

	// UTF-8 string comparisons
	if localTopMostHistory.EndpointID < incomingTopMostHistory.EndpointID {
		return false, -1
	}
	if localTopMostHistory.EndpointID > incomingTopMostHistory.EndpointID {
		return false, 1
	}

	if len(noteConflicts) == len(incomingNoteConflicts) {

		if len(noteConflicts) > 0 {

			for index := 0; index < len(noteConflicts); index++ {
				localConflictNote := noteConflicts[index]
				matchingConflict := false

				for index2 := 0; index2 < len(incomingNoteConflicts); index2++ {
					incomingConflictNote := incomingNoteConflicts[index2]
					_, compareResult := CompareModified(incomingConflictNote, localConflictNote)
					if compareResult == 0 {
						matchingConflict = true
						break
					}
				}

				if !matchingConflict {
					return true, 0
				}
			}
		}

		return false, 0

	}

	return true, 0

}

// IsSubsumedBy determines whether or not this Note was subsumed by changes to another
func IsSubsumedBy(xnote note.Note, incomingNote note.Note) bool {

	noteHistories := copyOrCreateNonblankHistory(xnote.Histories)
	incomingNoteHistories := copyOrCreateNonblankHistory(incomingNote.Histories)
	noteConflicts := copyOrCreateBlankConflict(xnote.Conflicts)
	incomingNoteConflicts := copyOrCreateBlankConflict(incomingNote.Conflicts)

	isSubsumed := false
	localTopMostHistory := noteHistories[0]

	for index := 0; index < len(incomingNoteHistories); index++ {
		incomingHistory := incomingNoteHistories[index]

		if isHistorySubsumedBy(localTopMostHistory, incomingHistory) {
			isSubsumed = true
			break
		}

	}

	if !isSubsumed {
		for index := 0; index < len(incomingNoteConflicts); index++ {
			incomingConflict := incomingNoteConflicts[index]
			if IsSubsumedBy(xnote, incomingConflict) {
				isSubsumed = true
				break
			}
		}
	}

	if !isSubsumed {
		return false
	}

	for index := 0; index < len(noteConflicts); index++ {
		isSubsumed = false

		localConflict := noteConflicts[index]
		if IsSubsumedBy(localConflict, incomingNote) {
			isSubsumed = true
			break
		}

		for index2 := 0; index2 < len(incomingNoteConflicts); index2++ {
			incomingConflict := incomingNoteConflicts[index2]
			if IsSubsumedBy(localConflict, incomingConflict) {
				isSubsumed = true
				break
			}
		}

		if !isSubsumed {
			return false
		}
	}

	return true
}

// Copy a list of conflicts, else if blank create a new one that is truly blank, with 0 items
func copyOrCreateBlankConflict(conflictsToCopy *[]note.Note) []note.Note {
	if conflictsToCopy != nil {
		return *conflictsToCopy
	}
	return []note.Note{}
}

// Copy a history, else if blank create a new one that is truly blank, with 0 items
func copyOrCreateBlankHistory(historiesToCopy *[]note.History) []note.History {
	if historiesToCopy != nil {
		return *historiesToCopy
	}
	return []note.History{}
}

// Copy a history, else if blank create a new entry with exactly 1 item
func copyOrCreateNonblankHistory(historiesToCopy *[]note.History) []note.History {
	if historiesToCopy != nil {
		return *historiesToCopy
	}
	histories := []note.History{}
	histories = append(histories, NewHistory("", 0, "", 0))
	return histories
}

// NewHistory creates a history entry for a Note being modified
func NewHistory(endpointID string, when int64, where string, sequence int32) note.History {

	newHistory := note.History{}
	newHistory.EndpointID = endpointID

	if when == 0 {
		newHistory.When = createNoteTimestamp(endpointID)
	} else {
		newHistory.When = when
	}

	if where == "" {
		// On the service we don't have a location
		newHistory.Where = ""
	} else {
		newHistory.Where = where
	}

	newHistory.Sequence = sequence

	return newHistory
}

// Determine whether or not a Note's history was subsumed by changes to another
func isHistorySubsumedBy(thisHistory note.History, incomingHistory note.History) bool {

	Subsumed := (thisHistory.EndpointID == incomingHistory.EndpointID) && (incomingHistory.Sequence >= thisHistory.Sequence)

	return Subsumed

}

// Merge the contents of two Notes
func Merge(localNote *note.Note, incomingNote *note.Note) note.Note {

	//  Create flattened array for local note & conflicts
	clonedLocalNote := copyNote(localNote)
	localNotes := copyOrCreateBlankConflict(clonedLocalNote.Conflicts)
	clonedLocalNote.Conflicts = nil
	localNotes = append(localNotes, clonedLocalNote)

	//  Create flattened array for incoming note & conflicts
	clonedIncomingNote := copyNote(incomingNote)
	incomingNotes := copyOrCreateBlankConflict(clonedIncomingNote.Conflicts)
	clonedIncomingNote.Conflicts = nil
	incomingNotes = append(incomingNotes, clonedIncomingNote)

	//  Remove duplicates & subsumed notes - also get the winner
	mergeResultNotes := []note.Note{}
	winnerNote := mergeNotes(&localNotes, &incomingNotes, &mergeResultNotes, nil)
	winnerNote = mergeNotes(&incomingNotes, &localNotes, &mergeResultNotes, winnerNote)
	if len(mergeResultNotes) == 1 {
		return *winnerNote
	}

	//  Reconstruct conflicts for item
	winnerNoteConflicts := copyOrCreateBlankConflict(nil)
	for index := 0; index < len(mergeResultNotes); index++ {
		mergeResultNote := mergeResultNotes[index]
		_, compareResult := CompareModified(*winnerNote, mergeResultNote)
		if 0 == compareResult {
			continue
		}
		winnerNoteConflicts = append(winnerNoteConflicts, mergeResultNote)
	}
	winnerNote.Conflicts = &winnerNoteConflicts

	// Done
	return *winnerNote
}

// The bulk of processing for merging two sets of Notes into a single result set
func mergeNotes(outerNotes *[]note.Note, innerNotes *[]note.Note, mergeNotes *[]note.Note, winnerNote *note.Note) *note.Note {

	for outerNotesIndex := 0; outerNotesIndex < len(*outerNotes); outerNotesIndex++ {
		outerNote := (*outerNotes)[outerNotesIndex]
		outerNoteSubsumed := false

		for innerNotesIndex := 0; innerNotesIndex < len(*innerNotes); innerNotesIndex++ {
			innerNote := (*innerNotes)[innerNotesIndex]

			//  Note can be specially flagged due to subsumption below, so check for it
			if innerNote.Updates == -1 {
				continue
			}

			// See if the outer note is subsumed by any changes in the inner notes
			outerNoteHistories := copyOrCreateNonblankHistory(outerNote.Histories)
			innerNoteHistories := copyOrCreateNonblankHistory(innerNote.Histories)
			outerHistory := outerNoteHistories[0]
			for historyIndex := 0; historyIndex < len(innerNoteHistories); historyIndex++ {
				innerHistory := innerNoteHistories[historyIndex]
				if outerHistory.EndpointID == innerHistory.EndpointID {
					outerNoteSubsumed = (innerHistory.Sequence >= outerHistory.Sequence)
					break
				}
			}
			if outerNoteSubsumed {
				break
			}
		}

		if outerNoteSubsumed {
			//  Place a special flag on the note to indicate that it has been subsumed
			(*outerNotes)[outerNotesIndex].Updates = -1
			continue
		}

		// Free the conflicts on this outer note, and append it to the merge note list
		(*outerNotes)[outerNotesIndex].Conflicts = nil
		(*mergeNotes) = append((*mergeNotes), (*outerNotes)[outerNotesIndex])

		needToAssignWinner := (winnerNote == nil)
		if !needToAssignWinner {
			_, compareResult := CompareModified(*winnerNote, (*outerNotes)[outerNotesIndex])
			needToAssignWinner = (-1 == compareResult)
		}
		if needToAssignWinner {
			winnerNote = &outerNote
		}
	}

	return winnerNote
}

// copyNote duplicates a Note
func copyNote(xnote *note.Note) note.Note {

	newNote := note.Note{}
	newNote.Updates = xnote.Updates
	newNote.Deleted = xnote.Deleted
	newNote.Sent = xnote.Sent
	newNote.Body = xnote.Body
	newNote.Payload = xnote.Payload

	if xnote.Histories != nil {
		newHistories := copyOrCreateBlankHistory(xnote.Histories)
		newNote.Histories = &newHistories
	}

	if xnote.Conflicts != nil {
		newConflicts := copyOrCreateBlankConflict(xnote.Conflicts)
		newNote.Conflicts = &newConflicts
	}

	return newNote
}

// Return the current timestamp to be used for "when", in milliseconds
var lastTimestamp int64

func createNoteTimestamp(endpointID string) int64 {

	// Return the later of the current time (in seconds) and the last one that we handed out
	thisTimestamp := time.Now().UTC().UnixNano() / 1000000000
	if thisTimestamp <= lastTimestamp {
		lastTimestamp++
		thisTimestamp = lastTimestamp
	} else {
		lastTimestamp = thisTimestamp
	}

	return thisTimestamp

}

// compareNoteTimestamps is a standard Compare function
func compareNoteTimestamps(left int64, right int64) int {
	if left < right {
		return -1
	} else if left > right {
		return 1
	}
	return 0
}
