module Views.Shared.SilencePreview exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, div, text, strong, p)
import Html.Attributes exposing (class)
import Utils.Types exposing (ApiData(Initial, Success, Loading, Failure))
import Views.Shared.AlertListCompact
import Utils.Views exposing (loading)


view : ApiData (List Alert) -> Html msg
view alertsResponse =
    case alertsResponse of
        Success alerts ->
            if List.isEmpty alerts then
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text "No silenced alerts" ] ] ]
            else
                div [ class "w-100" ]
                    [ p [] [ strong [] [ text "Silenced alerts:" ] ]
                    , Views.Shared.AlertListCompact.view alerts
                    ]

        Initial ->
            text ""

        Loading ->
            loading

        Failure e ->
            div [ class "alert alert-warning" ] [ text e ]
