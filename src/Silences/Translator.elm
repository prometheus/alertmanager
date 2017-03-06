module Silences.Translator exposing (translator)

import Silences.Types exposing (Msg(..), SilencesMsg, OutMsg(NewUrl, UpdateFilter, UpdateCurrentTime))
import Utils.Types exposing (Filter)
import Time


type alias TranslationDictionary msg =
    { onInternalMessage : SilencesMsg -> msg
    , onNewUrl : String -> msg
    , onUpdateFilter : Filter -> String -> msg
    , onUpdateCurrentTime : Time.Time -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onNewUrl, onUpdateFilter, onUpdateCurrentTime } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (NewUrl url) ->
            onNewUrl url

        ForParent (UpdateFilter filter string) ->
            onUpdateFilter filter string

        ForParent (UpdateCurrentTime time) ->
            onUpdateCurrentTime time
