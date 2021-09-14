module Views.Shared.AlertCompact exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Dict
import Html exposing (Html, div, table, text)
import Html.Attributes exposing (class, style)
import Utils.Views exposing (labelButton)
import Views.Shared.Alert exposing (annotation, annotationsButton, generatorUrlButton, titleView)
import Views.Shared.Types exposing (Msg)


view : Maybe String -> GettableAlert -> Html Msg
view activeAlertId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> Dict.toList
                |> List.partition (Tuple.first >> (==) "alertname")
                |> (\( a, b ) -> a ++ b)
                |> List.map (\( a, b ) -> String.join "=" [ a, b ])
    in
    div
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style "position" "static"
        , class "border-0 p-0 mb-4"
        ]
        [ div
            [ class "w-100 mb-2 d-flex" ]
            [ titleView alert
            , if Dict.size alert.annotations > 0 then
                annotationsButton activeAlertId alert

              else
                text ""
            , case alert.generatorURL of
                Just url ->
                    generatorUrlButton url

                Nothing ->
                    text ""
            ]
        , if activeAlertId == Just alert.fingerprint then
            table
                [ class "table w-100 mb-1" ]
                (List.map annotation <| Dict.toList alert.annotations)

          else
            text ""
        , div [] (List.map (labelButton Nothing) ungroupedLabels)
        ]
