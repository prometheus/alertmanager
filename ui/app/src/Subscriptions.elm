module Subscriptions exposing (subscriptions)

import Types exposing (Model, Msg(MsgForAlertList, MsgForSilenceList, Noop), Route(AlertsRoute, SilenceListRoute))
import Views.AlertList.Types exposing (AlertListMsg(FetchAlertGroups))
import Views.SilenceList.Types exposing (SilenceListMsg(FetchSilences))
import Time


subscriptions : Model -> Sub Msg
subscriptions model =
    Time.every (30 * Time.second)
        (\_ ->
            case model.route of
                AlertsRoute _ ->
                    MsgForAlertList FetchAlertGroups

                SilenceListRoute _ ->
                    MsgForSilenceList FetchSilences

                _ ->
                    Noop
        )
