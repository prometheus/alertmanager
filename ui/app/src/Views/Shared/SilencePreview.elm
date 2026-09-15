module Views.Shared.SilencePreview exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Html exposing (Html, div, p, strong, text)
import Html.Attributes exposing (class)
import Utils.Types exposing (ApiData(..))
import Utils.Views exposing (loading)
import Views.Shared.AlertListCompact
import Views.Shared.Types exposing (Msg)


view : Maybe String -> ApiData (List GettableAlert) -> Html Msg
view activeAlertId alertsResponse =
    case alertsResponse of
        Success alerts ->
            if List.isEmpty alerts then
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text "No affected alerts" ] ] ]

            else
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text ("Affected alerts: " ++ String.fromInt (List.length alerts)) ] ]
                    , Views.Shared.AlertListCompact.view activeAlertId alerts
                    ]

        Initial ->
            text ""

        Loading ->
            loading

        Failure e ->
            div [ class "alert alert-warning" ] [ text e ]
