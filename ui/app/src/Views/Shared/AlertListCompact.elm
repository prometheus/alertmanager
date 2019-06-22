module Views.Shared.AlertListCompact exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Html exposing (Html, div)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact
import Views.Shared.Types exposing (Msg)


view : Maybe String -> List GettableAlert -> Html Msg
view activeAlertId alerts =
    List.map (Views.Shared.AlertCompact.view activeAlertId) alerts
        |> div [ class "pa0 w-100" ]
