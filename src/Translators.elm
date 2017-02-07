module Translators exposing (..)

import Alerts.Types
import Alerts.Translator exposing (translator)
import Types exposing (Msg(Alerts, CreateSilenceFromAlert, UpdateLoading))


alertTranslator : Alerts.Types.Msg -> Msg
alertTranslator =
    translator
        { onInternalMessage = Alerts
        , onSilenceFromAlert = CreateSilenceFromAlert
        , onUpdateLoading = UpdateLoading
        }
