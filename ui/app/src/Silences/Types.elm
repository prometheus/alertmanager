module Silences.Types exposing (Silence, nullSilence, nullMatcher, nullTime, SilenceId)

import Alerts.Types exposing (Alert)
import Utils.Types exposing (Matcher, ApiData, ApiResponse(Success))
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
    , silencedAlerts = Success []
    }


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


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
    , silencedAlerts : ApiData (List Alert)
    }


type alias SilenceId =
    String
