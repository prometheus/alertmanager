module Views.SilenceForm.Types exposing (SilenceFormMsg(..))

import Silences.Types exposing (Silence)
import Utils.Types exposing (Matcher, ApiData)
import Time


type SilenceFormMsg
    = AddMatcher Silence
    | NewDefaultTimeRange Time.Time
    | CreateSilence Silence
    | FetchSilence String
    | NewSilence
    | SilenceFetch (ApiData Silence)
    | UpdateCreatedBy Silence String
    | DeleteMatcher Silence Matcher
    | UpdateDuration Silence String
    | UpdateEndsAt Silence String
    | UpdateMatcherName Silence Matcher String
    | UpdateMatcherRegex Silence Matcher Bool
    | UpdateMatcherValue Silence Matcher String
    | UpdateStartsAt Silence String
    | SilenceCreate (ApiData String)
    | UpdateComment Silence String
