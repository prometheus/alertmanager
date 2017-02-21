module Alerts.Translator exposing (translator)

import Alerts.Types exposing (Msg(..), Alert, AlertsMsg, OutMsg(..))
import Utils.Types exposing (Filter)


type alias TranslationDictionary msg =
    { onInternalMessage : AlertsMsg -> msg
    , onSilenceFromAlert : Alert -> msg
    , onUpdateFilter : Filter -> String -> msg
    , onNewUrl : String -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onSilenceFromAlert, onUpdateFilter, onNewUrl } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (SilenceFromAlert alert) ->
            onSilenceFromAlert alert

        ForParent (UpdateFilter filter string) ->
            onUpdateFilter filter string

        ForParent (NewUrl string) ->
            onNewUrl string
