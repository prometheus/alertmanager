module Silences.Types exposing
    ( Silence
    , SilenceId
    , State(..)
    , Status
    , nullMatcher
    , nullSilence
    , nullSilenceStatus
    , stateToString
    )

import Time exposing (Posix)
import Utils.Types exposing (Matcher)


nullSilence : Silence
nullSilence =
    { id = ""
    , createdBy = ""
    , comment = ""
    , startsAt = Time.millisToPosix 0
    , endsAt = Time.millisToPosix 0
    , updatedAt = Time.millisToPosix 0
    , matchers = [ nullMatcher ]
    , status = nullSilenceStatus
    }


nullSilenceStatus : Status
nullSilenceStatus =
    { state = Expired
    }


nullMatcher : Matcher
nullMatcher =
    Matcher False "" ""


type alias Silence =
    { id : SilenceId
    , createdBy : String
    , comment : String
    , startsAt : Posix
    , endsAt : Posix
    , updatedAt : Posix
    , matchers : List Matcher
    , status : Status
    }


type alias Status =
    { state : State
    }


type State
    = Active
    | Pending
    | Expired


stateToString : State -> String
stateToString state =
    case state of
        Active ->
            "active"

        Pending ->
            "pending"

        Expired ->
            "expired"


type alias SilenceId =
    String
