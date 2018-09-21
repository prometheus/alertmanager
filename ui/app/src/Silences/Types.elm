module Silences.Types exposing
    ( nullMatcher
    , nullSilence
    , nullSilenceStatus
    , stateToString
    )

import Data.Matcher exposing (Matcher)
import Data.Matchers exposing (Matchers)
import Data.Silence exposing (Silence)
import Data.SilenceStatus exposing (SilenceStatus, State(..))
import Time exposing (Posix)


nullSilence : Silence
nullSilence =
    { id = Nothing
    , createdBy = ""
    , comment = ""
    , startsAt = Time.millisToPosix 0
    , endsAt = Time.millisToPosix 0
    , updatedAt = Nothing
    , matchers = nullMatchers
    , status = Nothing
    }


nullSilenceStatus : SilenceStatus
nullSilenceStatus =
    { state = Expired
    }


nullMatchers : Matchers
nullMatchers =
    [ nullMatcher ]


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


stateToString : State -> String
stateToString state =
    case state of
        Active ->
            "active"

        Pending ->
            "pending"

        Expired ->
            "expired"
