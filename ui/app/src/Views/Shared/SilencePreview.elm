module Views.Shared.SilencePreview exposing (view)

import Silences.Types exposing (Silence)
import Html exposing (Html, div, text)
import Utils.Types exposing (ApiResponse(Success, Loading, Failure))
import Views.Shared.AlertListCompact
import Utils.Views exposing (error, loading)


view : Silence -> Html msg
view s =
    case s.silencedAlerts of
        Success alerts ->
            if List.isEmpty alerts then
                div [] [ text "No matches" ]
            else
                div [] [ Views.Shared.AlertListCompact.view alerts ]

        Loading ->
            loading

        Failure e ->
            error e
