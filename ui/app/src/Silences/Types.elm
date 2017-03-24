module Silences.Types exposing (Silence, nullSilence, nullMatcher, nullDuration, nullTime, SilenceId)

import Utils.Date
import Alerts.Types exposing (AlertGroup)
import Utils.Types exposing (Duration, Time, Matcher, ApiData, ApiResponse(Success))


nullSilence : Silence
nullSilence =
    { id = ""
    , createdBy = ""
    , comment = ""
    , startsAt = nullTime
    , endsAt = nullTime
    , duration = nullDuration
    , updatedAt = nullTime
    , matchers = [ nullMatcher ]
    , silencedAlertGroups = Success []
    }


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


nullDuration : Duration
nullDuration =
    Utils.Date.duration 0


nullTime : Time
nullTime =
    Utils.Date.fromTime 0


type alias Silence =
    { id : SilenceId
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , duration : Duration
    , updatedAt : Time
    , matchers : List Matcher
    , silencedAlertGroups : ApiData (List AlertGroup)
    }


type alias SilenceId =
    String
