module Silences.Types exposing (..)

import Http
import Utils.Types exposing (Time, Matcher, Filter, ApiData)
import Utils.Date
import Time
import ISO8601


type alias Silence =
    { id : String
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , updatedAt : Time
    , matchers : List Matcher
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


type SilencesMsg
    = DeleteMatcher Silence Matcher
    | AddMatcher Silence
    | UpdateMatcherName Silence Matcher String
    | UpdateMatcherValue Silence Matcher String
    | UpdateMatcherRegex Silence Matcher Bool
    | UpdateEndsAt Silence String
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
    Silence "" "" "" nullTime nullTime nullTime [ nullMatcher ]


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


nullTime : Time
nullTime =
    let
        epochString =
            ISO8601.toString Utils.Date.unixEpochStart
    in
        Time Utils.Date.unixEpochStart epochString True
