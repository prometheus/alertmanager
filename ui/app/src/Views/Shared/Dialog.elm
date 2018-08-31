module Views.Shared.Dialog exposing (Config, view)

import Html exposing (Html, button, div, text)
import Html.Attributes exposing (class, style)
import Html.Events exposing (onClick)


type alias Config msg =
    { header : Html msg
    , body : Html msg
    , footer : Html msg
    , onClose : msg
    }


view : Maybe (Config msg) -> Html msg
view maybeConfig =
    case maybeConfig of
        Nothing ->
            div []
                [ div [ class "modal" ] []
                , div [ class "modal-backdrop" ] []
                ]

        Just { onClose, body, footer, header } ->
            div []
                [ div [ class "modal in" ]
                    [ div [ class "modal-dialog" ]
                        [ div [ class "modal-content" ]
                            [ div [ class "modal-header" ]
                                [ button
                                    [ class "close"
                                    , onClick onClose
                                    ]
                                    [ text "x" ]
                                , header
                                ]
                            , div [ class "modal-body" ] [ body ]
                            , div [ class "modal-footer" ] [ footer ]
                            ]
                        ]
                    ]
                , div [ class "modal-backdrop in" ] []
                ]
