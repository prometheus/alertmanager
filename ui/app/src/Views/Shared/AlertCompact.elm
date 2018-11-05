module Views.Shared.AlertCompact exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, a, button, div, i, li, span, table, td, text, th, tr)
import Html.Attributes exposing (class, href, style)
import Utils.Views exposing (labelButton)
import Views.Shared.Alert exposing (annotation, annotationsButton, generatorUrlButton, titleView)
import Views.Shared.Types exposing (Msg(..))


view : Maybe String -> Alert -> Html Msg
view activeAlertId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> List.partition (Tuple.first >> (==) "alertname")
                |> (\( a, b ) -> (++) a b)
                |> List.map (\( a, b ) -> String.join "=" [ a, b ])
    in
    li
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style "position" "static"
        , class "align-items-start list-group-item border-0 p-0 mb-4"
        ]
        [ div
            [ class "w-100 mb-2 d-flex align-items-start" ]
            [ titleView alert
            , if List.length alert.annotations > 0 then
                annotationsButton activeAlertId alert

              else
                text ""
            , generatorUrlButton alert.generatorUrl
            ]
        , if activeAlertId == Just alert.id then
            table
                [ class "table w-100 mb-1" ]
                (List.map annotation alert.annotations)

          else
            text ""
        , div [] (List.map (labelButton Nothing) ungroupedLabels)
        ]
