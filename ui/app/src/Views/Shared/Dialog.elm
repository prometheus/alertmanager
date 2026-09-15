module Views.Shared.Dialog exposing (Config, view)

import Html exposing (Html, button, div, h5, text)
import Html.Attributes exposing (class, style)
import Html.Events exposing (onClick)


type alias Config msg =
    { title : String
    , body : Html msg
    , footer : Html msg
    , onClose : msg
    }


view : Maybe (Config msg) -> Html msg
view maybeConfig =
    case maybeConfig of
        Nothing ->
            div [ style "clip" "rect(0,0,0,0)", style "position" "fixed" ]
                [ div [ class "modal fade" ] []
                , div [ class "modal-backdrop fade" ] []
                ]

        Just { onClose, body, footer, title } ->
            div []
                [ div [ class "modal fade show", style "display" "block" ]
                    [ div [ class "modal-dialog modal-dialog-centered" ]
                        [ div [ class "modal-content" ]
                            [ div [ class "modal-header" ]
                                [ h5 [ class "modal-title" ] [ text title ]
                                , button
                                    [ class "close"
                                    , onClick onClose
                                    ]
                                    [ text "×" ]
                                ]
                            , div [ class "modal-body" ] [ body ]
                            , div [ class "modal-footer" ] [ footer ]
                            ]
                        ]
                    ]
                , div [ class "modal-backdrop fade show" ] []
                ]
