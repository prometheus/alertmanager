module Silences.Types exposing
    ( nullMatcher
    , nullSilence
    , nullSilenceStatus
    , stateToString
    )

import Data.Matcher exposing (Matcher)
import Data.PostableSilence exposing (PostableSilence)
import Data.SilenceStatus exposing (SilenceStatus, State(..))
import Time exposing (Posix)


nullSilence : PostableSilence
nullSilence =
    { id = Nothing
    , createdBy = ""
    , comment = ""
    , startsAt = Time.millisToPosix 0
    , endsAt = Time.millisToPosix 0
    , matchers = nullMatchers
    }


nullSilenceStatus : SilenceStatus
nullSilenceStatus =
    { state = Expired
    }


nullMatchers : List Matcher
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
