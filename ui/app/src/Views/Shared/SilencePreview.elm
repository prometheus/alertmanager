module Views.Shared.SilencePreview exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, div, p, strong, text)
import Html.Attributes exposing (class)
import Utils.Types exposing (ApiData(Failure, Initial, Loading, Success))
import Utils.Views exposing (loading)
import Views.Shared.AlertListCompact


view : ApiData (List Alert) -> Html msg
view alertsResponse =
    case alertsResponse of
        Success alerts ->
            if List.isEmpty alerts then
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text "No silenced alerts" ] ] ]
            else
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text ("Silenced alerts: " ++ toString (List.length alerts)) ] ]
                    , Views.Shared.AlertListCompact.view alerts
                    ]

        Initial ->
            text ""

        Loading ->
            loading

        Failure e ->
            div [ class "alert alert-warning" ] [ text e ]
