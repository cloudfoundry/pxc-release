// untested sections: 1

package gbytes

import (
	"fmt"
	"regexp"

	"github.com/onsi/gomega/format"
)

// Objects satisfying the BufferProvider can be used with the Say matcher.
type BufferProvider interface {
	Buffer() *Buffer
}

/*
Say is a Gomega matcher that operates on gbytes.Buffers:

	Expect(buffer).Should(Say("something"))

will succeed if the unread portion of the buffer matches the regular expression "something".

When Say succeeds, it fast forwards the gbytes.Buffer's read cursor to just after the successful match.
Thus, subsequent calls to Say will only match against the unread portion of the buffer

Say pairs very well with Eventually.  To assert that a buffer eventually receives data matching "[123]-star" within 3 seconds you can:

	Eventually(buffer, 3).Should(Say("[123]-star"))

Ditto with consistently.  To assert that a buffer does not receive data matching "never-see-this" for 1 second you can:

	Consistently(buffer, 1).ShouldNot(Say("never-see-this"))

In addition to bytes.Buffers, Say can operate on objects that implement the gbytes.BufferProvider interface.
In such cases, Say simply operates on the *gbytes.Buffer returned by Buffer()

If the buffer is closed, the Say matcher will tell Eventually to abort.
*/
func Say(expected string, args ...any) *sayMatcher {
	if len(args) > 0 {
		expected = fmt.Sprintf(expected, args...)
	}
	return &sayMatcher{
		re: regexp.MustCompile(expected),
	}
}

type sayMatcher struct {
	re              *regexp.Regexp
	receivedSayings []byte
}

func (m *sayMatcher) buffer(actual any) (*Buffer, bool) {
	var buffer *Buffer

	switch x := actual.(type) {
	case *Buffer:
		buffer = x
	case BufferProvider:
		buffer = x.Buffer()
	default:
		return nil, false
	}

	return buffer, true
}

func (m *sayMatcher) Match(actual any) (success bool, err error) {
	buffer, ok := m.buffer(actual)
	if !ok {
		return false, fmt.Errorf("Say must be passed a *gbytes.Buffer or BufferProvider.  Got:\n%s", format.Object(actual, 1))
	}

	didSay, sayings := buffer.didSay(m.re)
	m.receivedSayings = sayings

	return didSay, nil
}

func (m *sayMatcher) FailureMessage(actual any) (message string) {
	return fmt.Sprintf(
		"Got stuck at:\n%s\nWaiting for:\n%s",
		format.IndentString(string(m.receivedSayings), 1),
		format.IndentString(m.re.String(), 1),
	)
}

func (m *sayMatcher) NegatedFailureMessage(actual any) (message string) {
	return fmt.Sprintf(
		"Saw:\n%s\nWhich matches the unexpected:\n%s",
		format.IndentString(string(m.receivedSayings), 1),
		format.IndentString(m.re.String(), 1),
	)
}

func (m *sayMatcher) MatchMayChangeInTheFuture(actual any) bool {
	switch x := actual.(type) {
	case *Buffer:
		return !x.Closed()
	case BufferProvider:
		return !x.Buffer().Closed()
	default:
		return true
	}
}
