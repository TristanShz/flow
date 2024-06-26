package startsession_test

import (
	"testing"
	"time"

	startsession "github.com/TristanShz/flow/internal/application/usecases/flowsession/start"
	"github.com/TristanShz/flow/internal/domain/session"
	"github.com/TristanShz/flow/internal/tests"
)

func TestStartFlowSession_Success(t *testing.T) {
	f := tests.GetSessionFixture(t)

	startTime := time.Date(2024, time.April, 13, 17, 20, 0, 0, time.UTC)
	f.GivenNowIs(startTime)
	f.GivenPredefinedIdentifier("id-1")

	command := startsession.Command{
		Project: "Flow",
		Tags:    []string{"start"},
	}

	f.WhenStartingFlowSession(command)

	f.ThenSessionShouldBeSaved(session.Session{
		Id:        "id-1",
		StartTime: time.Date(2024, time.April, 13, 17, 20, 0, 0, time.UTC),
		Project:   "Flow",
		Tags:      []string{"start"},
	})
}

func TestStartFlowSession_AlreadyStarted(t *testing.T) {
	f := tests.GetSessionFixture(t)

	f.GivenSomeSessions([]session.Session{{
		Id:        "1",
		StartTime: time.Date(2024, time.April, 13, 17, 20, 0, 0, time.UTC),
		Project:   "Flow",
		Tags:      []string{"start"},
	}})

	command := startsession.Command{
		Project: "Flow",
		Tags:    []string{"already_started"},
	}

	f.WhenStartingFlowSession(command)

	f.ThenErrorShouldBe(startsession.ErrSessionAlreadyStarted)
}
