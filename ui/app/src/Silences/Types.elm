module Silences.Types
    exposing
        ( Silence
        , SilenceId
        , Status
        , State(Active, Pending, Expired)
        , nullSilence
        , nullSilenceStatus
        , nullMatcher
        , nullTime
        , stateToString
        )

import Utils.Types exposing (Matcher)
import Time exposing (Time)


nullSilence : Silence
nullSilence =
    { id = ""
    , createdBy = ""
    , comment = ""
    , startsAt = 0
    , endsAt = 0
    , updatedAt = 0
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


nullTime : Time
nullTime =
    0


type alias Silence =
    { id : SilenceId
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , updatedAt : Time
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
