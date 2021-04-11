module Utils.Views exposing
    ( apiData
    , checkbox
    , error
    , labelButton
    , linkifyText
    , loading
    , tab
    , validatedField
    , validatedTextareaField
    )

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onBlur, onCheck, onClick, onInput)
import Utils.FormValidation exposing (ValidatedField, ValidationState(..))
import Utils.String
import Utils.Types as Types


tab : tab -> tab -> (tab -> msg) -> List (Html msg) -> Html msg
tab tab_ currentTab msg content =
    li [ class "nav-item" ]
        [ if tab_ == currentTab then
            span [ class "nav-link active" ] content

          else
            button
                [ style "background" "transparent"
                , style "font" "inherit"
                , style "cursor" "pointer"
                , style "outline" "none"
                , class "nav-link"
                , onClick (msg tab_)
                ]
                content
        ]


labelButton : Maybe msg -> String -> Html msg
labelButton maybeMsg labelText =
    case maybeMsg of
        Nothing ->
            span
                [ class "btn btn-sm bg-faded btn-secondary mr-2 mb-2"
                , style "user-select" "text"
                , style "-moz-user-select" "text"
                , style "-webkit-user-select" "text"
                ]
                [ text labelText ]

        Just msg ->
            button
                [ class "btn btn-sm bg-faded btn-secondary mr-2 mb-2"
                , onClick msg
                ]
                [ span [ class "text-muted" ] [ text labelText ] ]


linkifyText : String -> List (Html msg)
linkifyText str =
    List.map
        (\result ->
            case result of
                Ok link ->
                    a [ href link, target "_blank" ] [ text link ]

                Err txt ->
                    text txt
        )
        (Utils.String.linkify str)


checkbox : String -> Bool -> (Bool -> msg) -> Html msg
checkbox name status msg =
    label [ class "f6 dib mb2 mr2 d-flex align-items-center" ]
        [ input [ type_ "checkbox", checked status, onCheck msg ] []
        , span [ class "pl-2" ] [ text <| " " ++ name ]
        ]


validatedField : (List (Attribute msg) -> List (Html msg) -> Html msg) -> String -> String -> (String -> msg) -> msg -> ValidatedField -> Html msg
validatedField htmlField labelText classes inputMsg blurMsg field =
    case field.validationState of
        Valid ->
            div [ class <| "d-flex flex-column form-group has-success " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , htmlField
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control form-control-success"
                    ]
                    []
                ]

        Initial ->
            div [ class <| "d-flex flex-column form-group " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , htmlField
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control"
                    ]
                    []
                ]

        Invalid error_ ->
            div [ class <| "d-flex flex-column form-group has-danger " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , htmlField
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control form-control-danger"
                    ]
                    []
                , div [ class "form-control-feedback" ] [ text error_ ]
                ]


validatedTextareaField : String -> String -> (String -> msg) -> msg -> ValidatedField -> Html msg
validatedTextareaField labelText classes inputMsg blurMsg field =
    let
        lineCount =
            String.lines field.value
                |> List.length
                |> clamp 3 15
    in
    case field.validationState of
        Valid ->
            div [ class <| "d-flex flex-column form-group has-success " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , textarea
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control form-control-success"
                    , rows lineCount
                    , disableGrammarly
                    ]
                    []
                ]

        Initial ->
            div [ class <| "d-flex flex-column form-group " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , textarea
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control"
                    , rows lineCount
                    , disableGrammarly
                    ]
                    []
                ]

        Invalid error_ ->
            div [ class <| "d-flex flex-column form-group has-danger " ++ classes ]
                [ label [] [ strong [] [ text labelText ] ]
                , textarea
                    [ value field.value
                    , onInput inputMsg
                    , onBlur blurMsg
                    , class "form-control form-control-danger"
                    , rows lineCount
                    , disableGrammarly
                    ]
                    []
                , div [ class "form-control-feedback" ] [ text error_ ]
                ]


apiData : (a -> Html msg) -> Types.ApiData a -> Html msg
apiData onSuccess data =
    case data of
        Types.Success payload ->
            onSuccess payload

        Types.Loading ->
            loading

        Types.Initial ->
            loading

        Types.Failure msg ->
            error msg


loading : Html msg
loading =
    div []
        [ span [] [ text "Loading..." ]
        ]


error : String -> Html msg
error err =
    div [ class "alert alert-warning" ]
        [ text (Utils.String.capitalizeFirst err) ]


disableGrammarly : Html.Attribute msg
disableGrammarly =
    attribute "data-gramm_editor" "false"
