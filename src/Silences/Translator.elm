module Silences.Translator exposing (translator)

import Silences.Types exposing (Silence, Msg(..), SilencesMsg, OutMsg(..))
import Utils.Types exposing (Filter)
import Time


type alias TranslationDictionary msg =
    { onInternalMessage : SilencesMsg -> msg
    , onNewUrl : String -> msg
    , onUpdateFilter : Filter -> String -> msg
    , onUpdateCurrentTime : Time.Time -> msg
    , onPreviewSilence : Silence -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onNewUrl, onUpdateFilter, onUpdateCurrentTime, onPreviewSilence } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (NewUrl url) ->
            onNewUrl url

        ForParent (UpdateFilter filter string) ->
            onUpdateFilter filter string

        ForParent (UpdateCurrentTime time) ->
            onUpdateCurrentTime time

        ForParent (PreviewSilence silence) ->
            onPreviewSilence silence
