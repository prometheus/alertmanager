module Silences.Types exposing (..)

import Utils.Types exposing (Time, Duration, Matcher, Filter, ApiData, ApiResponse(..))
import Utils.Date
import Time
import Alerts.Types exposing (AlertGroup)


type alias Silence =
    { id : String
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , duration : Duration
    , updatedAt : Time
    , matchers : List Matcher
    , silencedAlertGroups : ApiData (List AlertGroup)
    }


type Route
    = ShowSilences (Maybe String)
    | ShowNewSilence
    | ShowSilence String
    | ShowEditSilence String


type Msg
    = ForSelf SilencesMsg
    | ForParent OutMsg


type OutMsg
    = NewUrl String
    | UpdateFilter Filter String
    | UpdateCurrentTime Time.Time
    | PreviewSilence Silence


type SilencesMsg
    = DeleteMatcher Silence Matcher
    | AddMatcher Silence
    | UpdateMatcherName Silence Matcher String
    | UpdateMatcherValue Silence Matcher String
    | UpdateMatcherRegex Silence Matcher Bool
    | UpdateEndsAt Silence String
    | UpdateDuration Silence String
    | UpdateStartsAt Silence String
    | UpdateCreatedBy Silence String
    | UpdateComment Silence String
    | NewDefaultTimeRange Time.Time
    | Noop
    | SilenceCreate (ApiData String)
    | SilenceDestroy (ApiData String)
    | CreateSilence Silence
    | DestroySilence Silence
    | FilterSilences
    | SilenceFetch (ApiData Silence)
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | FetchSilence String
    | NewSilence


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
