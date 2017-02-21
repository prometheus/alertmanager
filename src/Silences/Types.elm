module Silences.Types exposing (..)

import Http
import Utils.Types exposing (Time, Matcher, Filter)
import Utils.Date
import Time
import ISO8601


type alias Silence =
    { id : Int
    , createdBy : String
    , comment : String
    , startsAt : Time
    , endsAt : Time
    , createdAt : Time
    , matchers : List Matcher
    }


type Msg
    = ForSelf SilencesMsg
    | ForParent OutMsg


type OutMsg
    = NewUrl String
    | UpdateFilter Filter String
    | ParseFilterText


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
    | SilenceCreate (Utils.Types.ApiData Int)
    | SilenceDestroy (Utils.Types.ApiData String)
    | CreateSilence Silence
    | DestroySilence Silence
    | FilterSilences


nullSilence : Silence
nullSilence =
    Silence 0 "" "" nullTime nullTime nullTime [ nullMatcher ]


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
