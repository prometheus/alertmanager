module Views.AutoComplete.Views exposing (view)

import Views.AutoComplete.Types exposing (Msg(..), Model)
import Html exposing (Html, Attribute, div, span, input, text, button, i, small, ul, li, a)
import Html.Attributes exposing (value, class, style, disabled, id, href)
import Html.Events exposing (onClick, onInput, on, keyCode)


view : Model -> Html Msg
view { list, fieldText, fields, matches } =
    let
        className =
            "has-success"

        isDisabled =
            False

        -- TODO: Interacting with autocomplete box
        -- * Clicking autocomplete suggestion
        -- * Scrolling with keyboard
        -- * Scrolling with keyboard + hitting enter
        -- * Creating autocomplete box
        -- TODO: Text needs to match one autocomplete field exactly
        onClickAttr =
            Just fieldText
                |> Maybe.map (AddField True)
                |> Maybe.withDefault Noop
                |> onClick
    in
        div
            [ class "row no-gutters align-items-start pb-4" ]
            (viewFields fields
                ++ [ div
                        [ class ("col form-group " ++ className)
                        , style
                            [ ( "padding", "5px" )
                            , ( "min-width", "200px" )
                            , ( "max-width", "500px" )
                            ]
                        ]
                        [ div [ class "input-group" ]
                            [ input
                                [ id "auto-complete-field"
                                , class "form-control"
                                , value fieldText
                                , onInput (UpdateFieldText)
                                ]
                                []
                            , span
                                [ class "input-group-btn" ]
                                [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "Add" ] ]
                            ]
                        , small [ class "form-text text-muted" ]
                            [ text "Label keys for grouping alerts"
                            ]
                        , ul [ class "list-group" ] (matchedFields matches)
                        ]
                   ]
            )


matchedFields : List String -> List (Html Msg)
matchedFields fields =
    fields
        |> List.map matchedField


matchedField : String -> Html Msg
matchedField field =
    li
        [ class "list-group-item"
        , onClick (AddField True field)
        ]
        [ text field ]


viewFields : List String -> List (Html Msg)
viewFields fields =
    fields
        |> List.map viewField


viewField : String -> Html Msg
viewField field =
    div [ class "col col-auto", style [ ( "padding", "5px" ) ] ]
        [ div [ class "btn-group" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteField True field)
                ]
                [ text field
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteField False field)
                ]
                [ text "×" ]
            ]
        ]
