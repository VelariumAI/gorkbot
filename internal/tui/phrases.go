package tui

import (
	"math/rand"
	"time"
)

var (
	// Standard loading phrases for regular Grok interactions
	standardPhrases = []string{
		"Reticulating splines...",
		"Consulting the oracle...",
		"Thinking...",
		"Pondering the mysteries...",
		"Warming up the neurons...",
		"Calculating probabilities...",
		"Shuffling bits...",
		"Charging flux capacitor...",
		"Aligning chakras...",
		"Summoning inspiration...",
		"Brewing thoughts...",
		"Tickling the electrons...",
		"Waking up the AI...",
		"Searching the void...",
		"Crunching numbers...",
		"Spinning up the hamster wheel...",
		"Asking the magic 8-ball...",
		"Consulting the stars...",
		"Invoking dark magic...",
		"Channeling creativity...",
		"Weaving arcane energies...",
		"Deciphering ancient glyphs...",
		"Gathering cosmic forces...",
		"Awakening the algorithm...",
		"Channeling the datasphere...",
		"Summoning digital spirits...",
		"Conjuring insights...",
		"Breathing life into tokens...",
		"Manifesting intelligence...",
		"Transcribing the infinite...",
	}

	// Consultant phrases for when Gemini is invoked
	consultantPhrases = []string{
		"Summoning the Architectural Spirits...",
		"Gemini is judging your code...",
		"Deep Deep Thought engaged...",
		"Asking the smart friend...",
		"Consulting the multimodal oracle...",
		"Gemini is consulting its 1M context window...",
		"Activating reasoning mode...",
		"Channeling the expert opinions...",
		"Requesting sage advice...",
		"Invoking the wisdom of the ancients...",
		"Gemini is thinking VERY hard...",
		"Analyzing with extreme prejudice...",
		"Consulting the Council of Experts...",
		"Gemini is having an existential crisis...",
		"Summoning architectural patterns...",
		"Asking 'What would Uncle Bob do?'...",
		"Consulting the design patterns handbook...",
		"Gemini is reading the docs you should have...",
		"Invoking best practices...",
		"Channeling senior engineer energy...",
		"Gemini is refactoring your question...",
		"Consulting the Sacred Texts (RFC specs)...",
		"Asking the rubber duck...",
		"Gemini is judging your variable names...",
		"Awakening the Grand Architect...",
		"Gemini is communing with the code gods...",
		"Scrying the solution space...",
		"Gemini is decoding the cosmic blueprint...",
		"Channeling multiverse knowledge...",
		"The Oracle is deliberating...",
		"Gemini is traversing the wisdom tree...",
		"Consulting the Elder Algorithms...",
	}

	// rng is initialized once with current time as seed
	rng *rand.Rand
)

func init() {
	// Initialize random number generator
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// GetRandomPhrase returns a random loading phrase based on the mode
// isConsultant determines whether to use consultant phrases (Gemini) or standard phrases (Grok)
func GetRandomPhrase(isConsultant bool) string {
	if isConsultant {
		return consultantPhrases[rng.Intn(len(consultantPhrases))]
	}
	return standardPhrases[rng.Intn(len(standardPhrases))]
}

// GetRandomPhraseWithFallback returns a random phrase with a fallback if slices are empty
func GetRandomPhraseWithFallback(isConsultant bool, fallback string) string {
	var phrases []string
	if isConsultant {
		phrases = consultantPhrases
	} else {
		phrases = standardPhrases
	}

	if len(phrases) == 0 {
		return fallback
	}

	return phrases[rng.Intn(len(phrases))]
}

// RotatePhrase returns a new random phrase different from the current one
func RotatePhrase(current string, isConsultant bool) string {
	var phrases []string
	if isConsultant {
		phrases = consultantPhrases
	} else {
		phrases = standardPhrases
	}

	if len(phrases) <= 1 {
		return current
	}

	// Keep trying until we get a different phrase
	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		newPhrase := phrases[rng.Intn(len(phrases))]
		if newPhrase != current {
			return newPhrase
		}
	}

	// Fallback: just return any random phrase
	return phrases[rng.Intn(len(phrases))]
}
