module Alerts.Translator exposing (translator)

import Alerts.Types exposing (Msg(..), Alert, AlertsMsg, OutMsg(..))


type alias TranslationDictionary msg =
    { onInternalMessage : AlertsMsg -> msg
    , onSilenceFromAlert : Alert -> msg
    , onUpdateLoading : Bool -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onSilenceFromAlert, onUpdateLoading } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (UpdateLoading loading) ->
            onUpdateLoading loading

        ForParent (SilenceFromAlert alert) ->
            onSilenceFromAlert alert
