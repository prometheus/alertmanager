module Views.Shared.AlertListCompact exposing (view)

import Data.GettableAlerts exposing (GettableAlerts)
import Html exposing (Html, div)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact
import Views.Shared.Types exposing (Msg)


view : Maybe String -> GettableAlerts -> Html Msg
view activeAlertId alerts =
    List.map (Views.Shared.AlertCompact.view activeAlertId) alerts
        |> div [ class "pa0 w-100" ]
