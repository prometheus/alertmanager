module Silences.Types exposing
    ( nullSilence
    , stateToString
    )

import Data.Matcher exposing (Matcher)
import Data.PostableSilence exposing (PostableSilence)
import Data.SilenceStatus exposing (State(..))
import Time


nullSilence : PostableSilence
nullSilence =
    { id = Nothing
    , createdBy = ""
    , comment = ""
    , startsAt = Time.millisToPosix 0
    , endsAt = Time.millisToPosix 0
    , matchers = nullMatchers
    , annotations = Nothing
    }


nullMatchers : List Matcher
nullMatchers =
    [ nullMatcher ]


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False (Just True)


stateToString : State -> String
stateToString state =
    case state of
        Active ->
            "active"

        Pending ->
            "pending"

        Expired ->
            "expired"
